// Package implv2 implements an [EditorTool.EditTool]
package implv2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"m/pkg/chat"
	"m/pkg/debugging"
	"m/pkg/global"
	"m/pkg/llm"
	"m/pkg/loaders/treesitters"
	"m/pkg/toolrunner"
	"m/pkg/types/chatmessages"
	"m/pkg/types/otool"

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

func (h haspath) IsToolArgument() {} // implements [chat.IsToolArgument]

type EditorTool struct {
	session *Session
	Ext     chat.Ext
}

func NewEditTool(session *Session) *EditorTool {
	if session == nil {
		session = &Session{}
	}
	edit := &EditorTool{
		session: session,
	}
	edit.session.editor = edit
	return edit
}

var MaxRounds = 7

func (s *Session) EditTool() otool.ExecFunc {
	return s.editor.EditTool
}

func NewResultMessage(threadID uuid.UUID, resp Result) chatmessages.ChatMessage {
	msg := *chatmessages.NewAux(resp)
	msg.From = EditorName
	msg.ThreadID = threadID
	return msg
}

var ConfirmAction = func(ctx context.Context, s string) error {
	return fmt.Errorf("there is no ConfirmAction function provided by the main package")
}

// EditTool main entrypoint for the EditorTool. Called by an LLM as a tool function with two parameters ("path" and "intent")
// Described in [../builtins.tools.json] embedded schema file, this is the Exec function called by [toolrunner.Runtools] package.
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
	// var outmsgs chatmessages.A
	// session := Session{ActiveFile: path, Intent: intent}
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
		//	stack = append(stack, chatmessages.A{
		//		m1,
		//	}...)
	}
	var operators []string
	creq := llm.NewChatRequest()
	var card string
	cl := debugging.NewConsoleLogger(global.LogWriter, global.LogWriter)
	rounds := 0
	numberFilesEdited := 0
	ext := e.Ext
	if e, ok := ctx.Value(toolrunner.CtxExt).(chat.Ext); ok {
		ext = e.FromA(ext)
	}
	ext.From = EditorName
	//e.Ext = ext

	handler := e.Ext.H
	if handler == nil {
		slog.Warn("making new handler")
		handler = GetHandler()
	}

	m1 := *chatmessages.NewSystemEphemeral(SystemInstruction() + "\n\n# Intent\n" + intent) // global no state
	m1.ThreadID = threadID
	handler.HandleToolOutputs(m1) // for display

	defer func() {
		session.LockStart = 0
		session.LockEnd = 0
	}()
	for ctx.Err() == nil {
		rounds++
		if rounds > MaxRounds {
			break
		}
		slog := slog.With("lock", fmt.Sprintf("%d-%d", session.LockStart, session.LockEnd))
		{

			newstate := *chatmessages.NewSystemEphemeral("")
			newstate.Content = strings.Join([]string{card, session.SystemMessage()}, "\n")
			newstate.ThreadID = threadID
			newusermsg := *chatmessages.NewFromRole("auto", "user", "What is the next action? Use a single edit_file protocol command.")
			newusermsg.ThreadID = threadID
			if numLocked := session.NumLinesLocked(); numLocked != 0 {
				if numLocked == 1 {
					newusermsg.Content = fmt.Sprintf("Line %d is now locked and targeted for replacement."+
						"Your reply will directly replace this line, and inserting additional lines if your reply contains more than 1 line. (or CANCEL to return to normal mode).",
						session.LockStart)
				} else {
					newusermsg.Content = fmt.Sprintf("Lines %d to %d are locked and targeted for replacement. "+
						"Your reply will directly replace this line range, inserting/deleting lines. (or CANCEL to return to normal mode).",
						session.LockStart, session.LockEnd)
				}
			}
			stack = append(stack, m1, newstate, newusermsg)
			handler.HandleToolOutputs(newstate)
			handler.HandleToolOutputs(newusermsg)
		}
		creq.Messages = stack.Raw()

		card = ""
		newmsg := chatmessages.NewFromRole(ext.From, "assistant", "")
		newmsg.ThreadID = threadID
		handlers := []chat.Handler{cl.HandleChatResponse, newmsg.GatherChatResponse}
		if handler != nil {
			handlers = append(handlers, handler.Handle) // stream to channel
		}
		if e.Ext.PreFunc != nil {
			if err := e.Ext.PreFunc(creq, ext.From); err != nil {
				return nil, err
			}
		} else {
			slog.Warn("no prefunc")
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
		creq.Think = false
		if session.NumLinesLocked() > 20 {
			creq.Think = "low"
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
			//if len(actionsParsed) != 0 {
			//			newmsg.Aux = actionsParsed
			//	}
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
				slog.Error("surgical edit failed", "err", err)
				errmsg := *chatmessages.NewThread(threadID, llm.Message{Role: "system"})
				errmsg.Content = fmt.Sprintf("ApplySurgicalEdit failed: %+v", err)
				stack = append(stack, errmsg)
				handler.HandleToolOutputs(errmsg)
				err = nil
				continue
			}
			var messagesGathered []string
			messagesGathered = append(messagesGathered, "Surgical edit applied successfully in-memory.")
			slog.Info("Editing File", "latest", messagesGathered[len(messagesGathered)-1])
			messagesGathered = append(messagesGathered, fmt.Sprintf("The file currently has %d lines.", len(session.Lines)))
			slog.Info("Editing File", "latest", messagesGathered[len(messagesGathered)-1])
			// in memory still. check lint
			langname := ""
			if !strings.HasSuffix(session.ActiveFile, ".txt") {
				var lang *treesitters.Lang
				lang, langname, _ = treesitters.Getlang(session.ActiveFile)
				if lang != nil {
					err = ValidateFileSyntax(string(session.Current()), langname, lang)
					if err != nil {
						messagesGathered = append(messagesGathered, fmt.Sprintf("By the way, you may have broken the syntax: %+v", err))
						slog.Info("Editing File", "latest", messagesGathered[len(messagesGathered)-1])
					}
					messagesGathered = append(messagesGathered, "Linter has no issues to report.")
				}

			}
			if AskBeforeWriting {
				err = ConfirmAction(ctx, fmt.Sprintf("EDITOR EDITING Lines:\n```%s\n%s\n```", langname, session.CurrentView()))
				if err != nil {
					messagesGathered = append(messagesGathered, fmt.Sprintf("The reviewer has denied this particular code and you should send a new better version now.\nError: %+v", err))
					slog.Info("Editing File", "latest", messagesGathered[len(messagesGathered)-1])
				}
			}
			if err == nil {
				slog.Info("WRITING FILE")
				err = os.WriteFile(session.ActiveFile, session.Current(), 0o600)
				if err != nil {
					return nil, stackerr.Errorf("WriteFile after editing failed: %w", err)
				}
				session.LockStart = 0
				session.LockEnd = 0
				messagesGathered = append(messagesGathered, "The file was successfully updated on the filesystem. Available operators: "+fmt.Sprintf("%q", AllOperations))
				slog.Info("Editing File", "latest", messagesGathered[len(messagesGathered)-1])
			}
			msg := *chatmessages.NewThread(threadID, llm.Message{Role: "system", Content: "ACTION LOG:\n" + strings.Join(messagesGathered, "\n")})
			stack = append(stack, msg)
			handler.HandleToolOutputs(msg)
			numberFilesEdited++
			break
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
			operators = append(operators, string(actionsParsed[i].Op))
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
		"You are no longer in the `edit_file` tool state machine.\n## Transcript of editor session (abbreviated, not raw)\n\n```\n%s\n```\n\n### Operation keywords previously used in this session: %q\n\nMost recent file view:\n\n```\n%s```\n",
		stack[startstacklen:].RemoveThoughts().FormatText("", ""), operators, session.CurrentView(),
	)}
	slog.Info("done editing", "out", toolout.Content)
	outmsg1 := *chatmessages.New(toolout)
	handler.HandleToolOutputs(outmsg1)
	outmsg1.ThreadID = threadID
	outmsgs := chatmessages.A{outmsg1}
	outmsgs = outmsgs.RemoveThoughts()
	_ = json.NewEncoder(global.LogWriter).Encode(stack[startstacklen:])
	slog.Info("editor returning messages", "n", len(outmsgs))
	_ = json.NewEncoder(global.LogWriter).Encode(outmsgs)
	session.LockStart = 0
	session.LockEnd = 0
	return outmsgs, nil
}

func ParseRange(s *Session, actionContent string) (int, int, error) {
	// Unpack coordinate arguments safely
	parts := strings.Split(actionContent, "-")
	var start, end int
	var err error

	if len(parts) == 0 || len(parts) > 2 {
		return 0, 0, errors.New("invalid coordinate format (need either <number> or <number>-<number>")
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
		return 0, 0, fmt.Errorf("coordinates out of file range boundaries (end line number %d is greater than number of lines %d)", end, len(s.Lines))
	}
	numLines := end - start + 1
	if numLines > MaxLinesLocked {
		return 0, 0, fmt.Errorf("coordinates are too big and the edit should be broken into smaller pieces (%d > max %d)", numLines, len(s.Lines))
	}
	return start, end, nil
}

// writeAtomic writes data to a temporary file and then renames it to the target path.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tempFile, err := os.CreateTemp(dir, "atomic-")
	if err != nil {
		return err
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.Write(data); err != nil {
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}

	return os.Rename(tempFile.Name(), path)
}