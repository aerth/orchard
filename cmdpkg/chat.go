package cmdpkg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"m/cmdpkg/spinner"
	"m/cmdpkg/templatedata"
	"m/pkg/chat"
	"m/pkg/chatmessages"
	"m/pkg/debugging"
	"m/pkg/global"
	"m/pkg/llm"
	"m/pkg/otool"
	"m/pkg/toolrunner"

	"github.com/aerth/mostly/stackerr"
	"github.com/google/uuid"
)

type EncoderFunc = func(v *chatmessages.ChatMessage) error

const (
	finishLoopTool     = otool.FinishLoopToolName //"finish_turn"
	finishLoopToolSpec = `{"function": {"name": "` + finishLoopTool + `", "description": "immediately finish this round, waiting for the user to respond"}, "parameters": {"required": [], "properties": {}}, "type": "function"}`
)

func NewEphemeralSystem(s string, v ...any) chatmessages.ChatMessage {
	cm := chatmessages.NewFromRole("", "system", fmt.Sprintf(s, v...))
	cm.ThreadID = global.Get().GetThreadID()
	cm.Ephemeral = true
	return *cm
}

type Session struct{}

func NewSession() *Session {
	return &Session{}
}

// ChatWith caller MUST call global.Cancel() after to prevent Context memory leaks
func ChatWith(ctx context.Context, stackInput chatmessages.ChatMessages, tools otool.Tools, e chat.Ext) error {
	slog := slog.With("ntools", len(tools))
	slog.Info("starting ChatWith")
	stack := make(chatmessages.ChatMessages, len(stackInput), 10000)
	copy(stack, stackInput)
	conf := global.Get()
	if len(stack) == 0 {
		slog.Warn("noop convo")
		return nil
	}
	var cl *debugging.ConsoleLogger
	{
		tw := global.LogWriter // thought writer
		out := global.LogWriter
		cl = debugging.NewConsoleLogger(out, tw)
	}
	var err error
	handlers := e.Handlers // unmodified before loop
	if e.ThreadID == uuid.Nil {
		e.ThreadID = global.Get().Get().ThreadID
	}
	noStore := e.ThreadID == uuid.Nil
	isAuto := false
Chat:
	for ctx.Err() == nil {
		slog.Info("Wow")
		for i := range stack {
			if stack[i].Role != "system" && stack[i].MessageID == uuid.Nil || (!noStore && stack[i].ThreadID == uuid.Nil) {
				// keep this to identify regressions
				return fmt.Errorf("detected a non-system (%s) message with no IDs: %d: %s", stack[i].Role, i, stack[i].String())
			}
		}
		top := stack.Top()
		if top.Role == "assistant" {
			slog.Error("already is assistant, cant generate", "top", top.String(), "roles", stack.FormatRoles())
			return nil
		}
		slog.Info("Chatting", "after", top)

		// this goroutine needs its own chat ptr for GatherChatResponse and then tool calls
		newcaller := chatmessages.NewFromRole(e.From, "assistant", "")
		newcaller.ThreadID = e.ThreadID
		//	newcaller.Ephemeral = true  // to prevent storing
		if true {
			var hh []llm.ChatStreamHandlerFunc
			if cl != nil {
				hh = append(hh, cl.HandleChatResponse)
			}
			if newcaller != nil {
				hh = append(hh, newcaller.GatherChatResponse)
			}
			spin := spinner.Start(ctx)
			if spin != nil {
				hh = append([]llm.ChatStreamHandlerFunc{spin.HandleChatResponse}, hh...)
			}
			e.Handlers = append(hh, handlers...)
			//	handl := []llm.ChatStreamHandlerFunc{ cl.HandleChatResponse, newcaller.GatherChatResponse}

			/*
			* CHAT LLM GENERATE
			*
			 */
			if cl != nil {
				cl.Reset() // nice thinking colors
			}
			e.Prepend = []llm.Message{{Role: "system", Content: global.GetSystemBase(templatedata.NewData(stack))}}
			e.Tools = tools.ToMarshaler()
			e.Tools = append(e.Tools, json.RawMessage(finishLoopToolSpec))
			for i := range e.Tools {
				_ = json.NewEncoder(global.LogWriter).Encode(e.Tools[i])
			}
			e.ShowRequests = conf.Get().ShowRequests
			err = chat.Chat(ctx, conf.GetThreadID(), stack.TopN(20), e)
			if spin != nil {
				_ = spin.HandleChatResponse(llm.ChatResponse{}) // ensure spinner is stopped first regardless of output
			}
			if summary, ok := newcaller.Summary.(string); ok {
				_, _ = global.LogWriter.Write([]byte("\n" + summary + "\n"))
			} else if err == nil {
				slog.Warn("no summary found")
			}
		}
		shouldBreak := err != nil

		if err != nil && !errors.Is(err, context.Canceled) {
			return stackerr.Errorf("gen: %w", err)
		}
		if errors.Is(err, context.Canceled) && newcaller.Content == "" {
			return err
		}
		if errors.Is(err, context.Canceled) {
			newcaller.Content += " (this reply was cancelled by user before it finished)"
		}
		newcaller.GenDuration = time.Since(newcaller.CreatedAt)
		if !shouldBreak {
			stack = append(stack, *newcaller) // for this thread only
			newcaller = &stack[len(stack)-1]  // fix ptr
		}
		if e.AfterGen != nil {
			if err := e.AfterGen(newcaller); err != nil {
				return err
			}
		}
		// handle tools
		if strings.HasPrefix(newcaller.Content, "SELECT_FILE ") {
			stack = append(stack, NewEphemeralSystem("Error: you must use edit_file tool to use SELECT_FILE and other operators"))
			continue
		}

		if shouldBreak || strings.HasSuffix(newcaller.Content, finishLoopTool) {
			return nil
		}

		if len(newcaller.ToolCalls) == 0 && strings.TrimSpace(newcaller.Content) == finishLoopTool {
			return nil
		}
		if len(newcaller.ToolCalls) == 0 { // cant reply to self, so this is a stop
			if isAuto {
				return nil // max 2 consecutive
			}
			stack = append(stack, NewEphemeralSystem("Quickly use %q tool to complete this round if there are no additional tool calls needed.", finishLoopTool))
			isAuto = true
			continue
		}
		isAuto = false
		if newcaller.ToolCalls.GetNames()[0] == finishLoopTool {
			return nil
		}
		//
		slog.Info("detected tool calls", "ncalls", len(newcaller.ToolCalls), "ok", ctx.Err() == nil)
		if e.PreTools != nil {
			err := e.PreTools(ctx, newcaller, newcaller.ToolCalls.GetNames())
			if err != nil {
				stack = append(stack, NewEphemeralSystem("PreTools error: %+v", err))
				continue
			}
		}
		// run the tool exec funcs (add Ext)
		ctx = context.WithValue(ctx, toolrunner.CtxExt, e)
		toolouts, err := toolrunner.Runtools(ctx, stack, tools, *newcaller)
		if len(toolouts) == 0 && err == nil {
			slog.Error("no tool outs and no err")
			return stackerr.Errorf("no tool outs and no error")
		}
		slog.Info("finished running tool calls",
			"ncalls", len(newcaller.ToolCalls), "err", err, "nresp", len(toolouts))

		// option: let the bot handle errors
		if WrapToolErrors &&
			err != nil &&
			!errors.Is(err, context.Canceled) &&
			!errors.Is(err, toolrunner.ErrFatalToolResult) {
			toolouts = append(toolouts, WrapToolError(err)...)
			err = nil
		}
		if err != nil {
			return stackerr.Errorf("%w", err)
		}
		if len(toolouts) == 0 {
			toolouts = append(toolouts, WrapToolError(fmt.Errorf("function %q gave no outputs, signalling an error in our orchestration loop", newcaller.ToolCalls.GetNames()))...)
		}
		for i := range toolouts {
			toolouts[i].ThreadID = e.ThreadID
			toolouts[i].From = e.From
			if toolouts[i].ToolName == "" {
				names := newcaller.ToolCalls.GetNames()
				slog.Warn("missing toolname, guessing", "i", i, "ncalls", len(names), "name", names[i%len(names)])
				toolouts[i].ToolName = names[i%len(names)]
			}
		}
		if e.NonStreamHandler != nil {
			for i := range toolouts {
				e.NonStreamHandler(toolouts[i]) // should handle a tool output similar to generated, such as displaying it
			}
		}
		stack = append(stack, toolouts...)
		continue Chat
	}

	return nil
}
