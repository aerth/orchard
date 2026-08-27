package cmdpkg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	"m/pkg/chat"
	"m/pkg/debugging"
	"m/pkg/global"
	"m/pkg/llm"
	"m/pkg/toolrunner"
	"m/pkg/types/chatmessages"
	"m/pkg/types/otool"
	"m/pkg/types/spinner"
	"m/pkg/types/templatedata"

	"github.com/aerth/mostly/stackerr"
	"github.com/google/uuid"
)

type EncoderFunc = func(v *chatmessages.ChatMessage) error

const (
	finishLoopTool = otool.FinishLoopToolName //"finish_turn"
)

var finishLoopToolSpec json.RawMessage = json.RawMessage(`{"function": {"name": "` + finishLoopTool + `", "description": "immediately finish this round, waiting for the user to respond"}, "parameters": {"required": [], "properties": {}}, "type": "function"}`)

func NewEphemeralSystem(s string, v ...any) chatmessages.ChatMessage {
	cm := chatmessages.NewFromRole("", "system", fmt.Sprintf(s, v...))
	cm.ThreadID = global.Get().GetThreadID()
	cm.Ephemeral = true
	return *cm
}

var staticMessage string

func SetStaticMessage(s string) {
	staticMessage = s
}

var nonudge = false

func GetSystemMessage(stack chatmessages.ChatMessages) (string, error) {
	if staticMessage != "" {
		return staticMessage, nil
	} else {
		x, err := global.GetSystemBase(templatedata.NewData(stack))
		if err != nil {
			return "", stackerr.Errorf("%w", err)
		}
		return x, nil
	}
}

// ChatWith is the main single-threaded fully-featured chat loop,
// it gathers generated responses, runs tools, and runs Ext hooks.
func ChatWith(ctx context.Context, stackInput chatmessages.ChatMessages, tools otool.Tools, e chat.Ext) error {
	slog := slog.With("ntools", len(tools))
	slog.Info("starting ChatWith")
	stack := make(chatmessages.ChatMessages, len(stackInput), 10000)
	copy(stack, stackInput)
	conf := global.Get()
	if len(stack) == 0 {
		slog.Warn("conversation is empty, (noop)")
		return nil
	}
	var cl *debugging.ConsoleLogger
	{
		cl = debugging.NewConsoleLogger(io.Discard)
	}
	var err error
	handlers := e.Handlers // unmodified before loop
	if e.ThreadID == uuid.Nil {
		e.ThreadID = global.Get().Get().ThreadID
	}
	noStore := e.ThreadID == uuid.Nil
	isAuto := false
	numRounds := 0

	restrict := global.Get().Get().ToolsLimited
	if restrict != nil {
		got := slices.DeleteFunc(slices.Clone(tools), func(t otool.Tool) bool {
			return !slices.Contains(restrict, t.Name)
		})
		if len(got) != len(tools) {
			slog.Error("removed tools", "was", len(tools), "now", len(got))
			tools = got
		}
	}
Chat:
	for ctx.Err() == nil {
		numRounds++
		slog = slog.With("round", numRounds)
		for i := range stack {
			if stack[i].Role != "system" && stack[i].MessageID == uuid.Nil || (!noStore && stack[i].ThreadID == uuid.Nil) {
				// keep this to identify regressions
				return fmt.Errorf("detected a non-system (%s) message with no IDs: %d: %s -- this means that a caller is forgetting to add a threadID but we cant assume in ChatWith", stack[i].Role, i, stack[i].String())
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

			var spr string
			spr, err = GetSystemMessage(stack)
			if err != nil {
				return stackerr.Errorf("%w", err)
			}
			e.Prepend = []llm.Message{{Role: "system", Content: spr}}

			e.ShowRequests = conf.Get().ShowRequests
			e.Tools = tools.ToMarshaler()
			e.Tools = append(e.Tools, finishLoopToolSpec)
			err = chat.ChatPlain(ctx, conf.GetThreadID(), stack.TopN(100), e)
			if spin != nil {
				_ = spin.HandleChatResponse(llm.ChatResponse{}) // ensure spinner is stopped first regardless of output
			}
			if summary, ok := newcaller.Summary.(string); ok {
				_, _ = global.LogWriter.Write([]byte("\n" + summary + "\n"))
			} else if err == nil {
				slog.Warn("no summary found")
			}
		}
		hasBreakAt := slices.Index(newcaller.ToolCalls.GetNames(), finishLoopTool)
		if err != nil && !errors.Is(err, context.Canceled) {
			return stackerr.Errorf("gen: %w", err)
		}
		if errors.Is(err, context.Canceled) && newcaller.Content == "" {
			if ctx.Err() != nil {
				err = context.Cause(ctx) // get our underlying error like 'ctrl+c pressed'
			}
			return err
		}
		if errors.Is(err, context.Canceled) {
			if ctx.Err() != nil {
				err = context.Cause(ctx) // get our underlying error like 'ctrl+c pressed'
			}
			newcaller.Content += " (this reply was cancelled by user before it finished: " + err.Error() + ")"
		}
		newcaller.GenDuration = time.Since(newcaller.CreatedAt)
		// handle tools
		if strings.HasPrefix(newcaller.Content, "SELECT_FILE ") { // TODO: add more operators like LOCK or GOTO use slices.Contains add [implv2.Operators] []string
			newcaller.Ephemeral = true
			stack = append(stack, NewEphemeralSystem("Error: you must use edit_file tool to use SELECT_FILE and other operators"))
			continue
		}

		stack = append(stack, *newcaller) // for this thread only
		newcaller = &stack[len(stack)-1]  // fix ptr for below
		switch {
		case len(newcaller.ToolCalls) == 1 && newcaller.Content == "" && newcaller.ToolCalls.GetNames()[0] == finishLoopTool:
			newcaller.Ephemeral = true
		case len(newcaller.ToolCalls) == 0 && strings.TrimSpace(newcaller.Content) == finishLoopTool:
			newcaller.Ephemeral = true
		case strings.HasSuffix(newcaller.Content, finishLoopTool):
			newcaller.Ephemeral = true
		case hasBreakAt >= 0:
			if hasBreakAt != 0 {
				newcaller.Ephemeral = true
			}
			newcaller.ToolCalls = slices.Delete(newcaller.ToolCalls, hasBreakAt, hasBreakAt)
		default:
		}
		if e.AfterGen != nil {
			if err := e.AfterGen(newcaller); err != nil {
				return err
			}
		}

			return nil
		}

		if len(newcaller.ToolCalls) == 0 && strings.TrimSpace(newcaller.Content) == finishLoopTool {
			return nil
		}
		if len(newcaller.ToolCalls) == 0 && !nonudge {
			slog.Info("nudging continue")
			newcaller.Ephemeral = true
			tmp := NewEphemeralSystem("This environment allows for tool calls. Use **" + finishLoopTool + "** tool call to break this loop, or use other tools to continue.")
			tmp.MaxDistance = 2
			stack = append(stack, tmp)
			continue
		}
		if len(newcaller.ToolCalls) == 0 { // cant reply to self, so this is a stop
			if hasBreakAt >= 0 || isAuto || nonudge {
				return nil // max 2 consecutive
			}
			stack = append(stack, NewEphemeralSystem("Quickly use %q tool to complete this round if there are no additional tool calls needed, or continue working on the task.", finishLoopTool))
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
				slog.Error("pre-tool confirmation failed", "err", err)
				stack = append(stack, NewEphemeralSystem("PreTools error: %+v", err))
				continue
			}
		}
		// run the tool exec funcs (add Ext)
		ctx = context.WithValue(ctx, toolrunner.CtxExt, e) // NOTE: toolrunner doesn't have our Ext pack yet.
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
	return context.Cause(ctx)
}
