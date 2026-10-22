// Package editv3 ...
package editv3

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"m/orchard/pkg/chat"
	"m/orchard/pkg/toolrunner"
	"m/orchard/pkg/types/chatmessages"
	"m/orchard/pkg/types/otool"

	"github.com/aerth/mostly/mllm"
)

type Mode int

const (
	ModeView Mode = iota
	ModeEdit
	ModeFinished
)

func (m Mode) String() string {
	switch m {
	case ModeView:
		return "view"
	case ModeEdit:
		return "edit"
	case ModeFinished:
		return "finished"
	}
	return "unknown"
}

type EditorSession struct {
	chat.ChatHandlerPlus
	chat.Ext
	mllm.E
	FilePath      string
	Original      []string
	Draft         []string
	Mode          Mode
	LockedStart   int
	LockedEnd     int
	LastEditStart int
	PendingText   []string
	Convo         chatmessages.ChatMessages
}

var (
	lockRegex = regexp.MustCompile(`^LOCK\s+(\d+)(?:-(\d+))?$`)
	gotoRegex = regexp.MustCompile(`^GOTO\s+(\d+)$`)
)

type ToolArg struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
}

type ToolWith struct {
	E          mllm.E
	Ext        chat.Ext
	GetHandler func() chat.ChatHandlerPlus
}

var _ otool.ExecFunc = ToolWith{}.Tool

// Tool editv3 is [otool.ExecFunc]
func (tw ToolWith) Tool(ctx context.Context, caller mllm.Message, i int) (chatmessages.ChatMessages, error) {
	var call chatmessages.Call[ToolArg]
	err := json.Unmarshal(caller.ToolCalls[i], &call)
	if err != nil {
		return nil, err
	}
	args := call.Function.Arguments
	b, err := os.ReadFile(args.Path)
	if err != nil {
		return nil, err
	}

	ext := tw.Ext
	if e, ok := ctx.Value(toolrunner.CtxExt).(chat.Ext); ok {
		ext = e.FromA(ext)
	}
	ext.From = "editor"

	if len(ext.Handlers) == 0 {
		return nil, fmt.Errorf("editor has no stream handlers yet")
	}
	lines := strings.Split(string(b), "\n")
	j := tw.GetHandler()
	x := NewSession(j, tw.E, ext, args.Path, lines)
	convoStartLen := len(x.Convo)
	err = x.Run(ctx)
	a := x.Convo[convoStartLen:]
	if err != nil {
		return a, err
	}
	return a, nil
}

func NewSession(j chat.ChatHandlerPlus, e mllm.E, ext chat.Ext, path string, content []string) *EditorSession {
	if e.Model == "" {
		e = mllm.Default()
	}
	orig := make([]string, len(content))
	draft := make([]string, len(content))
	copy(orig, content)
	copy(draft, content)

	s := &EditorSession{
		ChatHandlerPlus: j,
		Ext:             ext,
		E:               e,
		FilePath:        path,
		Original:        orig,
		Draft:           draft,
		Mode:            ModeView,
	}
	//s.appendSys(sysMsg)
	return s
}

func (s *EditorSession) Run(ctx context.Context) error {
	handlers := s.E.Handlers
	for s.Mode != ModeFinished {

		sysMsg := "You are in a FILE session for: " + s.FilePath + "\n" +
			"Strict operational rules:\n" +
			"- In View Mode, reply ONLY with one of: GOTO [line], LOCK [start][-end], or EXIT.\n" +
			"- Do NOT wrap code or commands in backticks or markdown fences.\n" +
			"- In Edit Mode, output replacement text line-by-line, ending with SAVE on its own line (or CANCEL to abort).\n\n" +
			"Current file view:\n" + s.renderWindow(1)

		s.E.Handlers = append(handlers, s.ChatHandlerPlus.Handle)
		resp, err := s.E.Get(ctx, append(s.Convo.Raw(), mllm.Message{Role: "system", Content: sysMsg}), false)
		if err != nil {
			return err
		}
		newmsg := *chatmessages.New(resp.Message)
		newmsg.From = "editor"
		s.Convo = append(s.Convo, newmsg)
		switch s.Mode {
		case ModeView:
			s.handleViewTurn(newmsg)
		case ModeEdit:
			s.handleEditTurn(newmsg)
		}
	}
	return nil
}

func (s *EditorSession) handleViewTurn(msg chatmessages.ChatMessage) {
	cmd := strings.TrimSpace(msg.Content)
	switch {
	case cmd == "EXIT":
		s.Mode = ModeFinished
		s.appendSys("Session closed.")
	case gotoRegex.MatchString(cmd):
		m := gotoRegex.FindStringSubmatch(cmd)
		line, _ := strconv.Atoi(m[1])
		s.appendSys(s.renderWindow(line))
	case lockRegex.MatchString(cmd):
		m := lockRegex.FindStringSubmatch(cmd)
		start, _ := strconv.Atoi(m[1])
		end := start
		if m[2] != "" {
			end, _ = strconv.Atoi(m[2])
		}
		if err := s.validateLock(start, end); err != nil {
			s.appendSys(err.Error())
			return
		}
		s.LockedStart, s.LockedEnd, s.PendingText, s.Mode = start, end, nil, ModeEdit
		s.appendSys(fmt.Sprintf("Locked %d-%d. Send replacement lines then SAVE or CANCEL.", start, end))
	default:
		s.appendSys("Invalid cmd. Use: GOTO [line], LOCK [start][-end], EXIT")
	}
}

func (s *EditorSession) handleEditTurn(msg chatmessages.ChatMessage) {
	lines := strings.Split(msg.Content, "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		switch line {
		case "SAVE":
			s.applyEdits(s.PendingText)
			s.LastEditStart = s.LockedStart
			s.resetLock()
			s.Mode = ModeView
			s.appendSys(fmt.Sprintf("Saved.\n%s", s.renderWindow(s.LastEditStart)))
			return
		case "CANCEL":
			s.resetLock()
			s.Mode = ModeView
			s.appendSys("Cancelled.")
			return
		default:
			s.PendingText = append(s.PendingText, raw)
		}
	}
	s.appendSys(fmt.Sprintf("%d lines buffered. Send SAVE or CANCEL.", len(lines)))
}

func (s *EditorSession) validateLock(start, end int) error {
	total := len(s.Draft)
	if start == 0 && end == 0 {
		return nil
	}
	if start < 1 || start > total || end < start || end > total {
		return fmt.Errorf("invalid range %d-%d for %d lines", start, end, total)
	}
	return nil
}

func (s *EditorSession) applyEdits(newLines []string) {
	start, end := s.LockedStart, s.LockedEnd
	if start == 0 {
		s.Draft = append(newLines, s.Draft...)
		return
	}
	if end > len(s.Draft) {
		end = len(s.Draft)
	}
	prefix := append([]string{}, s.Draft[:start-1]...)
	suffix := s.Draft[end:]
	res := make([]string, 0, len(prefix)+len(newLines)+len(suffix))
	res = append(res, prefix...)
	res = append(res, newLines...)
	s.Draft = append(res, suffix...)
}

func (s *EditorSession) renderWindow(target int) string {
	total := len(s.Draft)
	if total == 0 {
		return "[Empty File]"
	}
	if target < 1 {
		target = 1
	}
	if target > total {
		target = total
	}
	start, end := target-10, target+10
	if start < 1 {
		start = 1
	}
	if end > total {
		end = total
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s (%d-%d/%d) ---\n", s.FilePath, start, end, total)
	w := len(strconv.Itoa(end))
	for i := start; i <= end; i++ {
		fmt.Fprintf(&sb, "%*d | %s\n", w, i, s.Draft[i-1])
	}
	fmt.Fprintf(&sb, "Current Mode: %s", s.Mode.String())
	return sb.String()
}

func (s *EditorSession) appendSys(c string) {
	s.Convo = append(s.Convo, *chatmessages.NewChatMessage("system", c))
}

func (s *EditorSession) resetLock() {
	s.LockedStart, s.LockedEnd, s.PendingText = 0, 0, nil
}
