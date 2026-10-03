package cmdpkg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"m/orchard/pkg/chat"
	"m/orchard/pkg/global"
	"m/orchard/pkg/llm"
	"m/orchard/pkg/toolrunner"
	"m/orchard/pkg/types/chatmessages"
	"m/orchard/pkg/types/otool"
	"m/orchard/pkg/types/spinner"
	"m/orchard/pkg/types/templatedata"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/mostly/stackerr"
	"github.com/google/uuid"
)

type EncoderFunc = func(v *chatmessages.ChatMessage) error

const (
	finishLoopTool = otool.FinishLoopToolName //"finish_turn"
)

var finishLoopToolSpec json.RawMessage = json.RawMessage(`{"function": {"name": "` + finishLoopTool + `", "description": "calling this tool immediately finishes this turn, giving the (human) user an opportunity to send a message."}, "parameters": {"required": [], "properties": {}}, "type": "function"}`)

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

var badwordsimplv2 = []string{}

func SetBadWordsImplv2(a []string) {
	badwordsimplv2 = a
}

// ChatWith is the main single-threaded fully-featured chat loop,
// it gathers generated responses, runs tools, and runs Ext hooks.
func ChatWith(ctx context.Context, stackInput chatmessages.ChatMessages, tools otool.Tools, e chat.Ext) error {
	t1 := time.Now()
	slog := slog.With("ntools", len(tools))
	slog.Info("starting ChatWith")
	stack := make(chatmessages.ChatMessages, len(stackInput), 10000)
	copy(stack, stackInput)
	conf := global.Get()
	topRequest := stack.TopRole("user")
	if len(stack) == 0 {
		slog.Warn("conversation is empty, (noop)")
		return nil
	}
	var err error
	if e.ThreadID == uuid.Nil {
		e.ThreadID = global.Get().Get().ThreadID
	}
	noStore := e.ThreadID == uuid.Nil
	numRounds := 0

	restrict := global.Get().Get().ToolsLimited
	if len(restrict) != 0 {
		slog.Warn("")
		got := slices.DeleteFunc(slices.Clone(tools), func(t otool.Tool) bool {
			return !slices.Contains(restrict, t.Name)
		})
		if len(got) != len(tools) {
			slog.Error("removed tools", "was", len(tools), "now", len(got))
			tools = got
		} else {
			slog.Warn("removed no tools")
		}
	}
	var t2 time.Time
	handlers := e.Handlers // unmodified before loop
Chat:
	for ctx.Err() == nil {
		numRounds++
		slog = slog.With("round", numRounds)
		if !t2.IsZero() {
			slog.Info("starting new round", "stack", len(stack))
		}
		t2 = time.Now()
		for i := range stack {
			if stack[i].Role != "system" && stack[i].MessageID == uuid.Nil || (!noStore && stack[i].ThreadID == uuid.Nil) {
				// keep this to identify regressions
				if stack[i].Ephemeral {
					continue
				}
				return fmt.Errorf("detected a non-system (%s) message with no IDs: %d: %s --"+
					" this means that a caller is forgetting to add a threadID but we cant assume in ChatWith",
					stack[i].Role, i, stack[i].String())
			}
		}
		top := stack.Top()
		if top.Role == "assistant" {
			slog.Error("already is assistant, cant generate", "top", top.String(), "roles", stack.FormatRoles())
			return nil
		}

		// this goroutine needs its own chat ptr for GatherChatResponse and then tool calls
		newcaller := chatmessages.NewFromRole(e.From, "assistant", "")
		newcaller.ThreadID = e.ThreadID
		//	newcaller.Ephemeral = true  // to prevent storing
		if true {
			var hh []mllm.StreamHandler
			if newcaller != nil {
				hh = append(hh, newcaller.GatherChatResponse)
			}
			spin := spinner.Start(ctx)
			if spin != nil {
				hh = append([]mllm.StreamHandler{spin.HandleChatResponse}, hh...)
			}
			e.Handlers = append(hh, handlers...)
			slog.Info("Chatting", "after", top, "handlers", len(e.Handlers), "orig", len(handlers))
			//	handl := []llm.ChatStreamHandlerFunc{ cl.HandleChatResponse, newcaller.GatherChatResponse}

			/*
			* CHAT LLM GENERATE
			*
			 */

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
				_ = spin.HandleChatResponse(mllm.ChatResponse{}) // ensure spinner is stopped first regardless of output
			}
			if summary, ok := newcaller.Summary.(string); ok {
				_, _ = global.LogWriter.Write([]byte("\n" + summary + "\n"))
			} else if err == nil {
				slog.Warn("no summary found")
			}
		}
		if err != nil && !errors.Is(err, context.Canceled) {
			return stackerr.Errorf("gen: %w", err)
		}
		if errors.Is(err, context.Canceled) && newcaller.Content == "" {
			if ctx.Err() != nil {
				err = context.Cause(ctx) // get our underlying error like 'ctrl+c pressed'
			}
			return stackerr.Errorf("%w", err)
		}
		if errors.Is(err, context.Canceled) {
			if ctx.Err() != nil {
				err = context.Cause(ctx) // get our underlying error like 'ctrl+c pressed'
			}
			newcaller.Content += " (this reply was cancelled by user before it finished: " + err.Error() + ")"
		}
		newcaller.GenDuration = time.Since(newcaller.CreatedAt)
		// handle tools
		for _, op := range badwordsimplv2 {
			if strings.HasPrefix(newcaller.Content, string(op)) || strings.Contains(newcaller.Content, "\n"+string(op)+" ") { // TODO: add more operators like LOCK or GOTO use slices.Contains add [implv2.Operators] []string
				newcaller.Ephemeral = true
				stack = append(stack, NewEphemeralSystem("Attention! You must use edit_file tool to begin the file-editing session where you may use operator keywords."))
				continue
			}
		}

		toolCallBreakAt := slices.Index(otool.GetNames(newcaller.ToolCalls), finishLoopTool)
		if toolCallBreakAt >= 0 {
			if len(newcaller.ToolCalls) == 1 && toolCallBreakAt == 0 {
				newcaller.ToolCalls = nil
			} else {
				slog.Info("GOT BREAK", "at", toolCallBreakAt)
				time.Sleep(time.Second * 2)
				newcaller.ToolCalls = slices.Delete(newcaller.ToolCalls, toolCallBreakAt, toolCallBreakAt+1)
				if len(newcaller.ToolCalls) == 0 {
					newcaller.ToolCalls = nil
				}
			}
		}
		contentFinishLoopAt := strings.Index(newcaller.Content, finishLoopTool)
		if contentFinishLoopAt >= 0 {
			newcaller.Content = strings.TrimSpace(newcaller.Content[:contentFinishLoopAt])
		}
		if newcaller.Content == "" && len(newcaller.ToolCalls) == 0 {
			slog.Debug("got empty", "message_out", toJson(newcaller))
			return nil
		}
		stack = append(stack, *newcaller) // for this thread only
		newcaller = &stack[len(stack)-1]  // fix ptr for below
		if e.AfterGen != nil {
			if err := e.AfterGen(newcaller); err != nil {
				return stackerr.Errorf("%w", err)
			}
		}
		if contentFinishLoopAt >= 0 || toolCallBreakAt == 0 {
			slog.Info("Done with round", "d", time.Since(t1), "d2", time.Since(t2))
			return nil
		}
		switch {
		case contentFinishLoopAt >= 0:
			nonudge = true
		case len(newcaller.ToolCalls) == 1 && newcaller.Content == "" && otool.GetNames(newcaller.ToolCalls)[0] == finishLoopTool:
			nonudge = true
			newcaller.Ephemeral = true
		case strings.HasSuffix(newcaller.Content, finishLoopTool):
			nonudge = true
			newcaller.Ephemeral = true
		case toolCallBreakAt >= 0:
			nonudge = true
			if toolCallBreakAt != 0 {
				newcaller.Ephemeral = true
			}
		case toolCallBreakAt == 0: // first and only tool call. had content.
			return nil
		default:
		}
		if len(tools) != 0 && len(newcaller.ToolCalls) == 0 && !nonudge && toolCallBreakAt < 0 {
			if err := runQuickPreNudgeCheck(ctx, topRequest, *newcaller); err != nil {
				if errors.Is(err, ErrOK) {
					return nil
				}
				return stackerr.Errorf("%w", err)
			}
			slog.Info("nudging continue")
			newcaller.Ephemeral = true
			tmp := NewEphemeralSystem(
				"Attention! The assistant sent a text message, but it had no tool calls. " +
					"This environment allows for tool calls. The previous message was sent by the assistant successfully, and should not be re-submitted. The assistant *must* use **" + finishLoopTool + "** tool function call to *break this loop*, or use other tools to continue work. If there is no work to be done **in this turn**, the assistant must break the loop now by calling the " + finishLoopTool + " tool.")
			tmp.MaxDistance = 1
			stack = append(stack, tmp)
			continue
		}
		if len(newcaller.ToolCalls) == 0 { // cant reply to self, so this is a stop
			return nil
		}
		slog.Info("detected tool calls", "ncalls", len(newcaller.ToolCalls), "ok", ctx.Err() == nil, "names", otool.GetNames(newcaller.ToolCalls))
		if e.PreTools != nil {
			err := e.PreTools(ctx, newcaller, otool.GetNames(newcaller.ToolCalls))
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
			toolouts = append(toolouts, WrapToolError(fmt.Errorf("function %q gave no outputs, signalling an error in our orchestration loop", otool.GetNames(newcaller.ToolCalls)))...)
		}
		for i := range toolouts {
			toolouts[i].ThreadID = e.ThreadID
			toolouts[i].From = e.From
			if toolouts[i].ToolName == "" {
				names := otool.GetNames(newcaller.ToolCalls)
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

func runQuickPreNudgeCheck(ctx context.Context, topRequest, chatMessage chatmessages.ChatMessage) error {
	convo := chatmessages.ChatMessages{topRequest, chatMessage}
	cr := mllm.ChatResponse{}
	err := <-mllm.Default().StreamBG(ctx, mllm.Messages{
		{Role: "system", Content: "Below is a user request, and an assistant response. Determine if the chat loop should break. If the reply is sufficient, output only BREAK, otherwise output KEEP GOING"},
		{Role: "user", Content: convo},
	}, false, cr.Handler)
	if err != nil {
		return stackerr.Errorf("%w", err)
	}
	if strings.Contains(strings.ToUpper(cr.Message.Content), "BREAK") {
		return ErrOK
	}
	return nil
}

var ErrOK = errors.New("OK")

func toJson(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err.Error())
	}
	return b
}
