//go:build ignore

package editorv2

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/aerth/mostly/mllm"
)

type Mode int

const (
	ModeView Mode = iota
	ModeEdit
	ModeFinished
)

type EditorSession struct {
	FilePath      string
	Original      []string
	Draft         []string
	Mode          Mode
	LockedStart   int
	LockedEnd     int
	LastEditStart int
	PendingText   []string
	Convo         []Message
	mllm.E
}

func (s *EditorSession) Chat(ctx context.Context, convo []Message, think any) (Message, error) {
	respMsg, err := s.Get(ctx, s.Convo, false)
	return respMsg.Message, err
}

func (s *EditorSession) Run(ctx context.Context) error {
	for s.Mode != ModeFinished {
		respMsg, err := s.Get(ctx, s.Convo, false)
		if err != nil {
			return err
		}
		s.Convo = append(s.Convo, respMsg.Message)

		// 2. Route based on active mode
		switch s.Mode {
		case ModeView:
			s.handleViewTurn(respMsg.Message)
		case ModeEdit:
			s.handleEditTurn(respMsg.Message)
		}
	}
	return nil
}

type Message = mllm.Message

func (s *EditorSession) oldhandleEditTurn(msg Message) {
	cmd := strings.TrimSpace(msg.Content)

	switch cmd {
	case "SAVE":
		// Splice s.PendingText into s.Draft replacing range [LockedStart, LockedEnd]
		s.applyPendingEdits()

		// Reset lock state and return to View Mode
		editedStart := s.LockedStart
		s.LockedStart = 0
		s.LockedEnd = 0
		s.PendingText = nil
		s.Mode = ModeView
		// Render updated view window around edited lines so LLM sees shifted line numbers
		viewContent := s.renderWindow(editedStart)
		s.appendSystemMessage(fmt.Sprintf("Edits saved successfully. Current file view:\n\n%s", viewContent))
	case "CANCEL":
		// Discard buffered changes and reset locks
		s.LockedStart = 0
		s.LockedEnd = 0
		s.PendingText = nil
		s.Mode = ModeView
		s.appendSystemMessage("Edit cancelled. Returned to View Mode.")
	default:
		// Accumulate lines directly without stripping backticks/fences
		lines := strings.Split(msg.Content, "\n")
		s.PendingText = append(s.PendingText, lines...)

		s.appendSystemMessage(fmt.Sprintf(
			"Buffered %d line(s) for lines %d-%d. Send 'SAVE' to apply or 'CANCEL' to discard.",
			len(lines), s.LockedStart, s.LockedEnd,
		))
	}
}

// applyPendingEdits handles slice splicing for variable replacement lengths and prepend logic (line 0)
func (s *EditorSession) applyPendingEdits() {
	start := s.LockedStart
	end := s.LockedEnd

	// Edge case: LOCK 0 (prepends to beginning of file)
	if start == 0 {
		s.Draft = append(s.PendingText, s.Draft...)
		return
	}

	// 1-based bounds clamping
	if start < 1 {
		start = 1
	}
	if end > len(s.Draft) {
		end = len(s.Draft)
	}

	// 0-based slice indices: range to remove is [start-1 : end]
	prefix := s.Draft[:start-1]
	suffix := s.Draft[end:]

	// Reconstruct Draft with new lines spliced in place
	newDraft := make([]string, 0, len(prefix)+len(s.PendingText)+len(suffix))
	newDraft = append(newDraft, prefix...)
	newDraft = append(newDraft, s.PendingText...)
	newDraft = append(newDraft, suffix...)

	s.Draft = newDraft
}

func (s *EditorSession) applyEdits(newLines []string) {
	start, end := s.LockedStart, s.LockedEnd

	// Line 0 prepend edge-case
	if start == 0 {
		s.Draft = append(newLines, s.Draft...)
		return
	}

	// Clamp bounds (1-based)
	if start < 1 {
		start = 1
	}
	if end > len(s.Draft) {
		end = len(s.Draft)
	}

	// Slice splice: prefix + new + suffix
	prefix := append([]string{}, s.Draft[:start-1]...)
	suffix := s.Draft[end:]

	s.Draft = append(prefix, append(newLines, suffix...)...)
}

func (s *EditorSession) resetLock() {
	s.LockedStart = 0
	s.LockedEnd = 0
	s.PendingText = nil
}

const DefaultWindowSize = 20 // 10 lines above, 10 lines below target

func (s *EditorSession) renderWindow(targetLine int) string {
	totalLines := len(s.Draft)
	if totalLines == 0 {
		return "[Empty File]"
	}

	// Clamp target line within valid file bounds
	if targetLine < 1 {
		targetLine = 1
	}
	if targetLine > totalLines {
		targetLine = totalLines
	}

	halfWindow := DefaultWindowSize / 2
	start := max(targetLine-halfWindow, 1)
	end := min(targetLine+halfWindow, totalLines)

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s (Lines %d-%d of %d) ---\n", s.FilePath, start, end, totalLines)

	// Format line numbers consistently with right padding
	lineNumWidth := len(fmt.Sprintf("%d", end))
	formatStr := fmt.Sprintf("%%%dd | %%s\n", lineNumWidth)

	for i := start; i <= end; i++ {
		// Draft slice is 0-indexed, line numbers are 1-indexed
		fmt.Fprintf(&sb, formatStr, i, s.Draft[i-1])
	}

	return sb.String()
}

func (s *EditorSession) handleEditTurn(msg Message) {
	lines := strings.Split(msg.Content, "\n")

	for _, rawLine := range lines {
		trimmedline := strings.TrimSpace(rawLine)

		switch trimmedline {
		case "SAVE":
			s.applyEdits(s.PendingText)
			s.resetLock()
			s.Mode = ModeView
			// Auto-render updated file window around edited position
			view := s.renderWindow(s.LastEditStart)
			s.appendSystemMessage(fmt.Sprintf("Edits saved. File view around line %d:\n\n%s", s.LastEditStart, view))
			return

		case "CANCEL":
			s.resetLock()
			s.Mode = ModeView
			s.appendSystemMessage("Edit cancelled. Returned to View Mode.")
			return

		default:
			// Buffer the line into s.PendingText
			s.PendingText = append(s.PendingText, rawLine)
		}
	}

	// If SAVE/CANCEL was not hit in this turn, prompt LLM that lines were buffered
	s.appendSystemMessage(fmt.Sprintf("%d line(s) buffered. Output 'SAVE' to apply or 'CANCEL' to abort.", len(lines)))
}

func (s *EditorSession) appendSystemMessage(x string) {
	s.Convo = append(s.Convo, mllm.Message{Role: "system", Content: x})
}

var (
// lockRegex = regexp.MustCompile(`^LOCK\s+(\d+)(?:-(\d+))?$`)
// gotoRegex = regexp.MustCompile(`^GOTO\s+(\d+)$`)
)

func (s *EditorSession) handleViewTurn(msg Message) {
	cmd := strings.TrimSpace(msg.Content)

	switch {
	case cmd == "EXIT":
		s.Mode = ModeFinished
		s.appendSystemMessage("File editing session closed. Computing final diff...")

	case gotoRegex.MatchString(cmd):
		matches := gotoRegex.FindStringSubmatch(cmd)
		lineNum, _ := strconv.Atoi(matches[1])

		view := s.renderWindow(lineNum)
		s.appendSystemMessage(fmt.Sprintf("Moved to line %d:\n\n%s", lineNum, view))

	case lockRegex.MatchString(cmd):
		matches := lockRegex.FindStringSubmatch(cmd)
		start, _ := strconv.Atoi(matches[1])
		end := start

		// Range specified (e.g. LOCK 1-3)
		if matches[2] != "" {
			end, _ = strconv.Atoi(matches[2])
		}

		if err := s.validateLockBounds(start, end); err != nil {
			s.appendSystemMessage(fmt.Sprintf("Invalid LOCK range: %v", err))
			return
		}

		s.LockedStart = start
		s.LockedEnd = end
		s.PendingText = nil
		s.Mode = ModeEdit

		if start == 0 {
			s.appendSystemMessage("Locked line 0 (Prepend mode). Send replacement lines followed by 'SAVE' or 'CANCEL'.")
		} else {
			s.appendSystemMessage(fmt.Sprintf("Locked lines %d-%d. Send replacement lines followed by 'SAVE' or 'CANCEL'.", start, end))
		}

	default:
		s.appendSystemMessage("Unknown or invalid command in View Mode. Available commands: GOTO [line], LOCK [start][-end], EXIT")
	}
}

func (s *EditorSession) validateLockBounds(start, end int) error {
	total := len(s.Draft)

	// LOCK 0 is specifically allowed for prepending to the top of file
	if start == 0 && end == 0 {
		return nil
	}

	if start < 1 || start > total {
		return fmt.Errorf("start line %d out of bounds (file has %d lines)", start, total)
	}
	if end < start || end > total {
		return fmt.Errorf("end line %d out of bounds (must be between %d and %d)", end, start, total)
	}

	return nil
}

var (
	lockRegex = regexp.MustCompile(`^LOCK\s+(\d+)(?:-(\d+))?$`)
	gotoRegex = regexp.MustCompile(`^GOTO\s+(\d+)$`)
)

func (s *EditorSession) Run2(ctx context.Context) error {
	for s.Mode != ModeFinished {
		resp, err := s.Chat(ctx, s.Convo, false)
		if err != nil {
			return err
		}
		s.Convo = append(s.Convo, resp)
		switch s.Mode {
		case ModeView:
			s.handleViewTurn(resp)
		case ModeEdit:
			s.handleEditTurn(resp)
		}
	}
	return nil
}

func (s *EditorSession) h3andleViewTurn(msg Message) {
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

func (s *EditorSession) h3andleEditTurn(msg Message) {
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

func (s *EditorSession) applyE3dits() {
	start, end := s.LockedStart, s.LockedEnd
	if start == 0 {
		s.Draft = append(s.PendingText, s.Draft...)
		return
	}
	if start < 1 {
		start = 1
	}
	if end > len(s.Draft) {
		end = len(s.Draft)
	}
	prefix := append([]string{}, s.Draft[:start-1]...)
	suffix := s.Draft[end:]
	res := make([]string, 0, len(prefix)+len(s.PendingText)+len(suffix))
	res = append(res, prefix...)
	res = append(res, s.PendingText...)
	s.Draft = append(res, suffix...)
}

func (s *EditorSession) r8enderWindow(target int) string {
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
	sb.WriteString(fmt.Sprintf("--- %s (%d-%d/%d) ---\n", s.FilePath, start, end, total))
	w := len(strconv.Itoa(end))
	for i := start; i <= end; i++ {
		sb.WriteString(fmt.Sprintf("%*d | %s\n", w, i, s.Draft[i-1]))
	}
	return sb.String()
}

func (s *EditorSession) appendSys(c string) {
	s.Convo = append(s.Convo, Message{Role: "system", Content: c})
}

func (s *EditorSession) r8esetLock() {
	s.LockedStart, s.LockedEnd, s.PendingText = 0, 0, nil
}
