package implv2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"m/pkg/chat"
	"m/pkg/chatmessages"
	"m/pkg/debugging"
	"m/pkg/global"
	"m/pkg/llm"
	"m/pkg/loaders/treesitters"
	"m/pkg/otool"
	"m/pkg/toolrunner"

	"github.com/aerth/mostly/stackerr"
	"github.com/google/uuid"
)

type haspath struct {
	Path   string `json:"path,omitempty"`
	Intent string `json:"intent,omitempty"`
}

var AskBeforeWriting = false
var (
	GetHandler = func() chat.ChatHandlerPlus { return nil }
	GetSender  = func(Result) {}
	PreFunc    = func() {}
)

const EditorName = "editor_agent"

func (h haspath) IsToolArgument() {}

type EditorTool struct {
	session *Session
	Ext     chat.Ext
}

func NewEditTool() *EditorTool {
	edit := &EditorTool{
		session: &Session{},
	}
	edit.session.editor = edit
	return edit
}

var MaxRounds = 7

func (s *Session) EditTool() otool.ExecFunc {
	return s.editor.EditTool
}

func EditTool(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
	return nil, fmt.Errorf("wrong tool")
}

func (e *EditorTool) EditTool(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
	if ConfirmAction == nil {
		return nil, fmt.Errorf("no ConfirmAction available for EditTool")
	}
	// begin loop

	call, err := chat.ReadToolCall[haspath](ctx, caller, toolCallIndex)
	if err != nil {
		return nil, stackerr.Errorf("%w", err)
	}
	path := call.Function.Arguments.Path
	intent := call.Function.Arguments.Intent
	threadID := e.Ext.ThreadID
	if threadID == uuid.Nil {
		threadID = global.Get().Get().ThreadID
	}

	{
		errmsg := llm.Message{
			Role:     "system",
			Content:  "no output",
			ToolName: "edit_file", ToolCallID: caller.ToolCallID,
		}
		if path == "" {
			errmsg.Content = "Error: path required"
			return chatmessages.A{*chatmessages.NewFrom(EditorName, errmsg)}, nil
		}
		if len(intent) < 10 {
			errmsg.Content = "Error: intent should be descriptive"
			return chatmessages.A{*chatmessages.NewFrom(EditorName, errmsg)}, nil
		}
	}

	// begin loop
	var outmsgs chatmessages.A
	//session := Session{ActiveFile: path, Intent: intent}
	session := e.session
	session.ActiveFile = path
	session.Intent = intent
	slog := slog.With("filename", path)
	if e := session.Refresh().E; e != nil {
		slog.Error("refresher error", "err", e)
		return nil, stackerr.Errorf("%w", e)
	}
	stack, _ := ctx.Value(toolrunner.CtxStack).(chatmessages.ChatMessages)
	if l := len(stack); l > 4 {
		stack = stack[l-4:]
	}
	startstacklen := len(stack)
	if startstacklen == 0 {
		return nil, fmt.Errorf("no conversation memory")
	}
	{
		m1 := *chatmessages.NewSystemEphemeral(SystemInstruction() + "\n\n# Intent\n" + intent) // global no state
		m1.ThreadID = threadID
		stack = append(stack, chatmessages.A{
			m1,
		}...)
	}
	var operators []string
	creq := llm.NewChatRequest()
	var card string
	cl := debugging.NewConsoleLogger(global.LogWriter, global.LogWriter)
	rounds := 0
	numberFilesEdited := 0
	handler := GetHandler()
	ext := e.Ext
	if e, ok := ctx.Value(toolrunner.CtxExt).(chat.Ext); ok {
		ext = e
	}
	ext.From = EditorName
	e.Ext = ext

	for ctx.Err() == nil {
		rounds++
		if rounds > MaxRounds {
			break
		}
		slog := slog.With("lock", fmt.Sprintf("%d-%d", session.LockStart, session.LockEnd))
		tmpmsg := strings.Join([]string{card, session.SystemMessage()}, "\n")
		newstate := *chatmessages.NewSystemEphemeral(tmpmsg)
		newstate.ThreadID = threadID
		newusermsg := *chatmessages.NewFromRole("auto", "user", "What is the next action? Use a single operator command.")
		newusermsg.ThreadID = threadID
		if numLocked := session.NumLinesLocked(); numLocked != 0 {
			if numLocked == 1 {
				newusermsg.Content = fmt.Sprintf("Line %d is locked. Reply with only the updated lines (or CANCEL).", session.LockStart)
			} else {
				newusermsg.Content = fmt.Sprintf("Lines %d to %d are locked. Reply with only the updated lines (or CANCEL).", session.LockStart, session.LockEnd)
			}
		}
		stack = append(stack, newstate)
		handler.HandleToolOutputs(newstate)
		stack = append(stack, newusermsg)
		handler.HandleToolOutputs(newusermsg)
		creq.Messages = stack.Raw()
		card = ""
		log.Printf("TMEPMSG:\n%s", tmpmsg)
		creq.Messages = append(creq.Messages, llm.Message{Role: "system", Content: tmpmsg})
		newmsg := chatmessages.NewFromRole("editor_agent", "assistant", "")
		newmsg.ThreadID = threadID
		handlers := []chat.Handler{cl.HandleChatResponse, newmsg.GatherChatResponse}
		if handler != nil {
			handlers = append(handlers, handler.Handle)
		}
		if e.Ext.PreFunc != nil {
			if err := e.Ext.PreFunc(creq); err != nil {
				return nil, err
			}
		}

		/*
		 *
		 *  CHAT GEN for VIEW mode
		 *
		 */
		if handler != nil {
			if x, ok := handler.(interface {
				BeginStream() error
			}); ok {
				slog.Info("BeginStream")
				if err := x.BeginStream(); err != nil {
					return nil, err
				}
			}
		}
		err := llm.New().Chat(ctx, creq, handlers...)
		cl.Reset()
		if err != nil {
			operators = append(operators, err.Error())
			break
		}
		isEditMode := session.NumLinesLocked() > 0
		if !isEditMode && newmsg.Content == "" {
			operators = append(operators, "no output")
			break
		}
		var actionsParsed []Action
		if newmsg.Content != "" {
			actionsParsed, err = session.Parse(newmsg.Content)
			if rounds > 1 && errors.Is(err, ErrNot) && !isEditMode {
				// finished. latest was a "I have corrected the typos in `foo.txt` while maintaining the exact number of lines." message
				slog.Info("Finished editing", "message", newmsg.Content)
				stack = append(stack, *newmsg)
				break
			}
			if len(actionsParsed) != 0 {
				newmsg.Aux = actionsParsed
			}
			if len(actionsParsed) == 1 && actionsParsed[0].Op == OpDeleteLines {
				isEditMode = true
				n1, n2, err := ParseRange(session, actionsParsed[0].Content)
				slog.Warn("deleting lines", "from", n1, "to", n2, "err", err)
				if err != nil {
					errmsg := *chatmessages.NewThread(threadID, llm.Message{Role: "system"})
					errmsg.Content = fmt.Sprintf("Delete Lines failed: %+v", err)
					stack = append(stack, errmsg)
					handler.HandleToolOutputs(errmsg)
					continue
				}
				// trick to delete lines going directly to ApplySurgicalEdit
				session.LockStart = n1
				session.LockEnd = n2
				newmsg.Content = ""
			}
		}
		stack = append(stack, *newmsg)
		if isEditMode { // this is the raw reply that will replace the locked lines in memory
			err = session.ApplySurgicalEdit(strings.Split(newmsg.Content, "\n"))
			if err != nil {
				errmsg := *chatmessages.NewThread(threadID, llm.Message{Role: "system"})
				errmsg.Content = fmt.Sprintf("ApplySurgicalEdit failed: %+v", err)
				stack = append(stack, errmsg)
				handler.HandleToolOutputs(errmsg)
				continue
			}
			// in memory still. check lint
			if !strings.HasSuffix(session.ActiveFile, ".txt") {
				lang, langname, _ := treesitters.Getlang(session.ActiveFile)
				if lang != nil {
					err = ValidateFileSyntax(string(session.Current()), langname, lang)
					if err != nil {
						errmsg := *chatmessages.NewThread(threadID, llm.Message{Role: "system"})
						errmsg.Content = fmt.Sprintf("ValidateFileSyntax failed: %+v", err)
						stack = append(stack, errmsg)
						handler.HandleToolOutputs(errmsg)
						continue
					}
				}
			}
			if AskBeforeWriting {
				err = ConfirmAction(ctx, fmt.Sprintf("EDITOR EDITING Lines:\n```\n%s\n```", newmsg.Content))
				if err != nil {
					errmsg := *chatmessages.NewThread(threadID, llm.Message{Role: "system"})
					errmsg.Content = fmt.Sprintf("ConfirmAction failed: %+v", err)
					stack = append(stack, errmsg)
					handler.HandleToolOutputs(errmsg)
					continue
				}
			}
			slog.Info("WRITING FILE")
			err = os.WriteFile(session.ActiveFile, session.Current(), 0o600)
			if err == nil {
				session.LockStart = 0
				session.LockEnd = 0
				stack = append(stack, *chatmessages.NewThread(threadID, llm.Message{
					Role:    "system",
					Content: "The file edits were applied to the filesystem\n " + fmt.Sprintf("The file currently has %d lines.", len(session.Lines)),
				}))
				handler.HandleToolOutputs(stack[len(stack)-1])
				numberFilesEdited++
				continue
			}

		}
		if err != nil {
			slog.Error("parsing edit TODO", "err", err)
			msg := *chatmessages.NewSystemEphemeral("Error: " + err.Error() + "\n\nTry again or use CANCEL to return to conversation")
			msg.ThreadID = threadID
			stack = append(stack, msg)
			handler.HandleToolOutputs(msg)
			continue
		}
		responseWriter := GetSender
		for i := range actionsParsed {
			slog.Info("running action", "op", actionsParsed[i].Op)
			operators = append(operators, actionsParsed[i].Op)
			resp := session.RunAction(actionsParsed[i])
			if responseWriter == nil && handler != nil {
				handler.HandleToolOutputs(NewResultMessage(threadID, resp))
			}
			if responseWriter != nil {
				responseWriter(resp)
			}
			if resp.E != nil {
				operators = append(operators, fmt.Sprintf("ERROR: %+v", resp.E))
				break
			}

		}
	}

	toolout := llm.Message{Role: "tool", Content: fmt.Sprintf(
		"## Transcript of editor session (abbreviated, not raw)\n\n```\n%s\n```\n\n### Operation keywords found: %q\n\nMost recent file view:\n\n```\n%s```\n",
		stack[startstacklen:].RemoveThoughts().FormatText("", ""), operators, session.CurrentView(),
	)}
	outmsg1 := *chatmessages.New(toolout)
	outmsg1.ThreadID = threadID
	outmsgs = append(outmsgs, outmsg1)
	outmsgs = outmsgs.RemoveThoughts()
	_ = json.NewEncoder(global.LogWriter).Encode(stack[startstacklen:])
	slog.Info("editor returning messages", "n", len(outmsgs))
	_ = json.NewEncoder(global.LogWriter).Encode(outmsgs)
	return outmsgs, nil
}

func ParseRange(s *Session, actionContent string) (int, int, error) {
	// Unpack coordinate arguments safely
	parts := strings.Split(actionContent, "-")
	var start, end int
	var err error

	if len(parts) == 0 || len(parts) > 2 {
		return 0, 0, errors.New("invalid coordinate format")
	}
	l := len(parts)
	switch {
	case l == 0:
		return 0, 0, errors.New("invalid coordinate format: need '%d' or '%d-%d' format (not enough args)")
	case l > 2:
		return 0, 0, errors.New("invalid coordinate format: need '%d' or '%d-%d' format (too many args)")
	}

	start, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, errors.New("invalid start coordinate format: " + err.Error())
	}
	if l == 1 {
		end = start
	}
	if l == 2 {
		end, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, errors.New("invalid end coordinate format: " + err.Error())
		}
	}
	if start < 1 || end < start {
		return 0, 0, errors.New("coordinates out of file range boundaries (start)")
	}
	if end > len(s.Lines) {
		return 0, 0, fmt.Errorf("coordinates out of file range boundaries (end %d is greater than number of lines %d)", end, len(s.Lines))
	}
	if end-start > MaxLinesLocked {
		return 0, 0, fmt.Errorf("coordinates are too big and the edit should be broken into smaller pieces (%d > max %d)", (end+1)-start, len(s.Lines))
	}
	return start, end, nil
}

func NewResultMessage(threadID uuid.UUID, resp Result) chatmessages.ChatMessage {
	msg := *chatmessages.NewAux(resp)
	msg.From = EditorName
	msg.ThreadID = threadID
	return msg
}

var ConfirmAction = func(ctx context.Context, s string) error {
	return fmt.Errorf("there is no ConfirmAction function provided")
}
