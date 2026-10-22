// Package implv2 implements an [EditorTool.EditTool]
package implv2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"uuid"

	"m/orchard/pkg/chat"
	"m/orchard/pkg/global"
	"m/orchard/pkg/llm"
	"m/orchard/pkg/loaders/treesitters"
	"m/orchard/pkg/toolrunner"
	"m/orchard/pkg/types/chatmessages"
	"m/orchard/pkg/types/otool"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/mostly/stackerr"
)

type haspath struct {
	Path   string `json:"path,omitempty"`
	Intent string `json:"intent,omitempty"`
}

type EditorConfig struct {
	AskBeforeWriting bool
	GetHandler       func() chat.ChatHandlerPlus
	GetSender        func(Result)
	PreFunc          func(*mllm.ChatRequest, string) error
	ThreadID         uuid.UUID
}

var DefaultEditorConfig = &EditorConfig{
	AskBeforeWriting: false,
	GetHandler:       func() chat.ChatHandlerPlus { return nil },
	GetSender:        func(Result) {},
	PreFunc:          func(creq *mllm.ChatRequest, from string) error { return nil },
}

const EditorName = "editor_agent"

func (h haspath) IsToolArgument() {} // implements [chat.IsToolArgument]

type EditorTool struct {
	session *Session
	Ext     chat.Ext
	Cfg     *EditorConfig
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

var MaxRounds = 0 // unlimited 0

func (s *Session) EditTool() otool.ExecFunc {
	return s.editor.EditTool
}

func NewResultMessage(threadID uuid.UUID, resp Result) chatmessages.ChatMessage {
	msg := *chatmessages.NewAux(resp)
	msg.From = EditorName
	msg.ThreadID = threadID
	msg.Ephemeral = true
	return msg
}

var ConfirmAction = func(ctx context.Context, s string, pre string) error {
	return stackerr.Errorf("there is no ConfirmAction function provided by the main package")
}

// EditTool main entrypoint for the EditorTool. Called by an LLM as a tool function with two parameters ("path" and "intent")
// Described in [../builtins.tools.json] embedded schema file, this is the Exec function called by [toolrunner.Runtools] package.
func (e *EditorTool) EditTool(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
	ctx = global.NewCancel(ctx)
	if ConfirmAction == nil {
		return nil, stackerr.Errorf("no ConfirmAction available for EditTool")
	}
	// begin loop

	call, err := chat.ReadToolCall[haspath](ctx, caller, toolCallIndex)
	if err != nil {
		return nil, stackerr.Errorf("%w", err)
	}
	threadID := e.Ext.ThreadID
	if threadID == uuid.Nil() {
		threadID = e.Cfg.ThreadID
	}
	if threadID == uuid.Nil() {
		threadID = global.Get().Get().ThreadID
	}
	if threadID == uuid.Nil() {
		slog.Error("no thread id found anywhere") // todo ctx lol
		return nil, stackerr.Errorf("no thread id in EditTool")
	}
	got, err := e.editToolInner(ctx, caller, toolCallIndex, call, threadID)
	for i := range got {
		got[i].ThreadID = threadID
		if got[i].MessageID == uuid.Nil() {
			got[i].MessageID = uuid.New()
		}
	}
	if err != nil {
		return got, stackerr.Errorf("%w", err)
	}
	return got, err
}

func (e *EditorTool) editToolInner(ctx context.Context, caller llm.Message, toolCallIndex int, call chatmessages.Call[haspath], parentThreadID uuid.UUID) (chatmessages.ChatMessages, error) {
	path := call.Function.Arguments.Path
	intent := call.Function.Arguments.Intent
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
	session := e.session // reuse same session ptr
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
		return nil, stackerr.Errorf("no conversation memory")
	}
	{
		//	stack = append(stack, chatmessages.A{
		//		m1,
		//	}...)
	}
	var operators []string
	var card string
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
		handler = e.Cfg.GetHandler()
	}

	sessionThreadID := uuid.New() // new threadID for this work

	m1 := *chatmessages.NewSystemEphemeral(SystemInstruction() + "\n\n# Intent\n" + intent) // global no state
	m1.ThreadID = sessionThreadID
	handler.HandleToolOutputs(m1) // for display

	defer func() {
		session.LockStart = 0
		session.LockEnd = 0
	}()
	var filesModifiedList []string
	for ctx.Err() == nil {
		rounds++
		if MaxRounds != 0 && rounds > MaxRounds {
			slog.Warn("hit max rounds")
			break
		}
		slog := slog.With("lock", fmt.Sprintf("%d-%d", session.LockStart, session.LockEnd))
		{

			newstate := *chatmessages.NewSystemEphemeral("")
			newstate.Content = strings.Join([]string{card, session.SystemMessage()}, "\n")
			newstate.ThreadID = sessionThreadID
			newusermsg := *chatmessages.NewFromRole("auto", "system",
				fmt.Sprintf(
					"You are currently in the `edit_file` session, with no lines locked for editing."+
						"Use a single edit_file protocol command to proceed. Available commands: %q\nAvoid locking lines that we are not changing.", AllOperations))
			newusermsg.ThreadID = sessionThreadID
			if numLocked := session.NumLinesLocked(); numLocked != 0 {
				if numLocked == 1 {
					newusermsg.Content = fmt.Sprintf("Line %d is now locked and targeted for replacement.\n"+
						"Your reply will directly replace this line, and inserting additional lines if your reply contains more than 1 line. (or CANCEL to return to normal mode).",
						session.LockStart)
				} else {
					newusermsg.Content = fmt.Sprintf("Lines %d to %d are locked and targeted for replacement.\n"+
						"Your reply will directly replace this line range, inserting/deleting lines if your reply contains a different number of lines. (or CANCEL to return to normal mode).",
						session.LockStart, session.LockEnd)
				}
			}
			newusermsg.MaxDistance = 2
			stack = append(stack, m1, newstate, newusermsg)
			handler.HandleToolOutputs(newstate)
			handler.HandleToolOutputs(newusermsg)
		}
		card = ""
		newmsg := chatmessages.NewFromRole(ext.From, "assistant", "")
		newmsg.ThreadID = sessionThreadID
		pending := mllm.ChatResponse{}
		handlers := []chat.Handler{pending.Handler}
		if handler != nil {
			handlers = append(handlers, handler.Handle) // stream to channel
		}
		creq := &mllm.ChatRequest{
			Messages: stack.Raw(), // TODO: use ChatPlain !!!!!!!!!!!!!
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
			} else {
				slog.Error("not a BeginStream interface", "T", fmt.Sprintf("%T", handler))
			}
		} else {
			slog.Error("handler is nil!!")
		}
		creq.Think = &mllm.Think{
			Value: false,
		}
		if session.NumLinesLocked() > 20 {
			creq.Think.Value = "low"
		}
		err := mllm.Default().WithTools(ext.Tools...).Stream(ctx, creq.Messages, false, handlers...)
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
					errmsg := *chatmessages.NewThread(sessionThreadID, llm.Message{Role: "system"})
					errmsg.Content = fmt.Sprintf("Delete Lines failed: %+v", err)
					errmsg.ThreadID = sessionThreadID
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
		if ctx.Err() != nil {
			break
		}
		if isEditMode { // this is the raw reply that will replace the locked lines in memory
			err = session.ApplySurgicalEdit(strings.Split(newmsg.Content, "\n"))
			if err != nil {
				slog.Error("surgical edit failed", "err", err)
				if errors.Is(err, ErrKeywordCancelled) {
					// no longer in edit mode
					session.LockStart = 0
					session.LockEnd = 0
					continue
				}
				errmsg := *chatmessages.NewThread(sessionThreadID, llm.Message{Role: "system"})
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
			if e.Cfg.AskBeforeWriting {
				err = ConfirmAction(ctx, fmt.Sprintf("EDITOR EDITING Lines:\n```%s\n%s\n```", langname, session.CurrentView()), "")
				if err != nil {
					messagesGathered = append(messagesGathered, fmt.Sprintf("The reviewer has denied this particular code and you should send a new better version now.\nError: %+v", err))
					slog.Info("Editing File", "latest", messagesGathered[len(messagesGathered)-1])
				}
			}
			if err == nil {
				slog.Info("WRITING FILE")
				err = atomicWriteFile(session.ActiveFile, session.Current(), 0o600)
				if err != nil {
					return nil, stackerr.Errorf("WriteFile after editing failed: %w", err)
				}
				session.LockStart = 0
				session.LockEnd = 0
				messagesGathered = append(messagesGathered, "The file was successfully updated on the filesystem. Available operators: "+fmt.Sprintf("%q", AllOperations))
				slog.Info("Editing File", "latest", messagesGathered[len(messagesGathered)-1])
				numberFilesEdited++
				filesModifiedList = append(filesModifiedList, session.ActiveFile)
			}
			msg := *chatmessages.NewThread(sessionThreadID, llm.Message{Role: "system", Content: "ACTION LOG:\n" + strings.Join(messagesGathered, "\n")})
			stack = append(stack, msg)
			handler.HandleToolOutputs(msg)
			break
		}
		if err != nil {
			slog.Error("parsing edit TODO", "err", err)
			msg := *chatmessages.NewSystemEphemeral("Could not parse operator keyword: " + err.Error() + "\n\nTry again or use CANCEL to return to conversation")
			msg.MaxDistance = 2
			msg.ThreadID = sessionThreadID
			stack = append(stack, msg)
			handler.HandleToolOutputs(msg)
			continue
		}
		responseWriter := e.Cfg.GetSender
		for i := range actionsParsed {
			if ctx.Err() != nil {
				break
			}
			slog.Info("running action", "op", actionsParsed[i].Op)
			operators = append(operators, string(actionsParsed[i].Op))
			resp := session.ApplyAction(actionsParsed[i])
			if responseWriter == nil && handler != nil {
				handler.HandleToolOutputs(NewResultMessage(sessionThreadID, resp))
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

	newer := stack[startstacklen:].RemoveThoughts().TopN(20)

	FirstLineSystemOnly(newer)
	toolout := llm.Message{Role: "tool", ToolName: call.Function.Name, ToolCallID: call.ID, Content: fmt.Sprintf(
		"You have returned from the `edit_file` tool session. %d writes were completed. Files were modified: %q. You are no longer in the `edit_file` tool state machine.\n## Transcript of editor session (abbreviated, not raw)\n\n```\n%s\n```\n\n### Operation keywords previously used in this session: %q\n\nMost recent file view:\n\n```\n%s```\n",
		numberFilesEdited,
		filesModifiedList,
		newer.FormatTextIf(EditorName, EditorName), operators, session.CurrentView(),
	)}
	slog.Info("done editing", "out", toolout.Content)
	outmsg1 := *chatmessages.New(toolout)
	outmsg1.ThreadID = parentThreadID
	//handler.HandleToolOutputs(outmsg1)
	outmsgs := chatmessages.A{outmsg1}
	outmsgs = outmsgs.RemoveThoughts()
	slog.Info("editor returning messages", "n", len(outmsgs))
	if false {
		_ = json.NewEncoder(global.LogWriter).Encode(stack[startstacklen:])
		_ = json.NewEncoder(global.LogWriter).Encode(outmsgs)
	}
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
		return 0, 0, stackerr.Errorf("invalid coordinate format (need either <number> or <number>-<number>")
	}
	l := len(parts)
	switch {
	case l == 0:
		return 0, 0, stackerr.Errorf("invalid coordinate format: need '%%d' or '%%d-%%d' format (not enough args)")
	case l > 2:
		return 0, 0, stackerr.Errorf("invalid coordinate format: need '%%d' or '%%d-%%d' format (too many args)")
	}

	start, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, stackerr.Errorf("invalid start coordinate format: %w", err)
	}
	if l == 1 {
		end = start
	}
	if l == 2 {
		end, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, stackerr.Errorf("invalid end coordinate format: %w", err)
		}
	}
	if start < 1 || end < start {
		return 0, 0, stackerr.Errorf("coordinates out of file range boundaries (start)")
	}
	if end > len(s.Lines) {
		return 0, 0, stackerr.Errorf("coordinates out of file range boundaries (end line number %d is greater than number of lines %d)", end, len(s.Lines))
	}
	numLines := end - start + 1
	if numLines > MaxLinesLocked {
		return 0, 0, stackerr.Errorf("coordinates are too big (we want small edits only) and the edit should be broken into smaller pieces (%d > max %d)", numLines, MaxLinesLocked)
	}
	return start, end, nil
}

// atomicWriteFile writes data to a temporary file and then renames it to the target path.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tempFile, err := os.CreateTemp(dir, "atomic-")
	if err != nil {
		return stackerr.Errorf("%w", err)
	}
	if perm != 0o600 {
		if err := tempFile.Chmod(perm); err != nil {
			return stackerr.Errorf("%w", err)
		}
	}
	defer func() {
		_ = os.Remove(tempFile.Name()) // TODO: log err
	}()

	if _, err := tempFile.Write(data); err != nil {
		return stackerr.Errorf("%w", err)
	}
	if err := tempFile.Close(); err != nil {
		return stackerr.Errorf("%w", err)
	}

	err = os.Rename(tempFile.Name(), path)
	if err != nil {
		return stackerr.Errorf("%w", err)
	}
	return nil
}

func FirstLineSystemOnly(a chatmessages.ChatMessages) {
	for i := range a {
		if a[i].Role == "system" {
			a[i].Content = strings.Split(a[i].Content, "\n")[0]
		}
	}
}
