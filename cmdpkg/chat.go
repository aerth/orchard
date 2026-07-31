package cmdpkg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
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

// MasterSend if not nil, is used to 'finish' each message (assistant,system,tool)
var MasterSend EncoderFunc = nil // = func(v *chatmessages.ChatMessage) error {

const finishLoopTool = "finish_turn"

func errmsg(s string) chatmessages.ChatMessage {
	cm := chatmessages.NewFromRole("", "system", s)
	cm.ThreadID = global.Get().GetThreadID()
	cm.Ephemeral = true
	return *cm
}

type Session struct{}

func NewSession() *Session {
	return &Session{}
}

func ChatWith(ctx context.Context, stackInput chatmessages.ChatMessages, tools otool.Tools, e chat.Ext) error {
	slog := slog.With("ntools", len(tools))
	slog.Info("starting ChatWith")
	stack := stackInput
	showChar := false
	conf := global.Get()
	if len(stack) == 0 {
		slog.Warn("noop convo")
		return nil
	}
	var jsonout EncoderFunc
	var cl *debugging.ConsoleLogger
	{
		tw := global.LogWriter
		var out io.Writer = os.Stdout
		if MasterSend != nil {
			slog.Info("using MasterSend")
			out = global.LogWriter
			tw = global.LogWriter
			jsonout = MasterSend
		} else if conf.Get().JsonOutput {
			out = global.LogWriter
			tw = global.LogWriter
			enc := json.NewEncoder(os.Stdout).Encode
			jsonout = func(v *chatmessages.ChatMessage) error {
				return enc(v)
			}
		}
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
				return fmt.Errorf("detected a non-system (%s) message with no IDs: %d: %s", stack[i].Role, i, stack[i].String())
			}
		}
		sctx := global.NewCancel(ctx)
		top := stack.Top()
		if top.Role == "assistant" {
			slog.Error("already is assistant, cant generate", "top", top.String(), "roles", stack.FormatRoles())
			return nil
		}
		slog.Info("Chatting", "after", top)

		// this goroutine needs its own chat ptr for GatherChatResponse and then tool calls
		newcaller := chatmessages.NewFromRole(e.From, "assistant", "")
		newcaller.ThreadID = e.ThreadID
		newcaller.Ephemeral = true
		if true {
			var hh []llm.ChatStreamHandlerFunc
			if cl != nil {
				hh = append(hh, cl.HandleChatResponse)
			}
			if newcaller != nil {
				hh = append(hh, newcaller.GatherChatResponse)
			}
			spin := spinner.Start(sctx)
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

			e.ShowRequests = conf.Get().ShowRequests
			err = chat.Chat(sctx, conf.GetThreadID(), stack.TopN(20), e)
			if spin != nil {
				_ = spin.HandleChatResponse(llm.ChatResponse{}) // ensure spinner is stopped first regardless of output
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
		if errors.Is(err, context.Canceled) {
			newcaller.Content += " (this reply was cancelled by user before it finished)"
		}
		newcaller.GenDuration = time.Since(newcaller.CreatedAt)
		stack = append(stack, *newcaller) // for this thread only
		newcaller = &stack[len(stack)-1]  // fix ptr
		/*
			if e.Store != nil {
				// Add sets initial msg.DBID
				if err := e.Store.Add(context.WithoutCancel(ctx), e.ThreadID, newgenerated); err != nil {
					return stackerr.Errorf("%w", err)
				}
			}*/
		if e.AfterGen != nil {
			if err := e.AfterGen(newcaller); err != nil {
				return err
			}
		}
		if jsonout != nil {
			// jsonout encoder may modify the pointer (TODO: hook for confirmation/cancel)
			if err := jsonout(&stack[len(stack)-1]); err != nil {
				return stackerr.Errorf("%w", err)
			}
		}
		// handle tools
		if strings.HasPrefix(newcaller.Content, "SELECT_FILE ") {
			stack = append(stack, errmsg("Error: you must use edit_file tool to use SELECT_FILE and other operators"))
			continue
		}
		if len(newcaller.ToolCalls) == 0 { // cant reply to self, so this is a stop
			if isAuto {
				return nil
			}
			stack = append(stack, errmsg("To end this round, use the %q tool to send the previous message to the user. Otherwise, proceed.", finishLoopTool))
			continue
		}
		if newcaller.ToolCalls.GetNames()[0] == finishLoopTool {
			return nil
		}
		//
		slog.Info("detected tool calls", "ncalls", len(newcaller.ToolCalls))
		names := strings.Join(newcaller.ToolCalls.GetNames(), ",")
		if e.PreTools != nil {
			err := e.PreTools(ctx, newcaller, names)
			if err != nil {
				/*			errmsg := func(s string) chatmessages.ChatMessage {
							cm := chatmessages.NewFromRole("", "system", "Error: you must use edit_file tool to use SELECT_FILE and other operators")
							cm.Ephemeral = true
							return *cm
						}*/
				stack = append(stack, errmsg(err.Error()))
				continue
			}
		}
		// if e.PreTool(ctx, )
		// run the tool exec funcs
		ctx = context.WithValue(ctx, toolrunner.CtxExt, e)
		toolouts, err := toolrunner.Runtools(ctx, stack, tools, *newcaller)
		{
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
				toolouts = append(toolouts, WrapToolError(fmt.Errorf("no output"))...)
			}
			for i := range toolouts {
				toolouts[i].ThreadID = e.ThreadID
				toolouts[i].From = e.From
			}
		}
		/*
			if e.Store != nil {
				if err := e.Store.Add(ctx, conf.GetThreadID(), chat.Ptrs(toolouts)...); err != nil {
					return stackerr.Errorf("adding tool output to db: %w", err)
				}
			}*/
		{

			// TODO: remove MasterSend
			if MasterSend != nil || (!conf.IsHideToolOuts() && jsonout != nil) { // if external handled, send all
				for i := range toolouts {
					if err := jsonout(&toolouts[i]); err != nil {
						return stackerr.Errorf("%w", err)
					}
				}
			}
			if e.NonStreamHandler != nil {
				for i := range toolouts {
					e.NonStreamHandler(toolouts[i]) // should handle a tool output similar to generated, such as displaying it
				}
			}
			if !conf.IsHideToolOuts() && jsonout == nil {
				for i := range toolouts {
					if showChar {
						_, _ = fmt.Fprintf(os.Stderr, "TOOL> ")
					}
					_, _ = fmt.Fprintf(os.Stderr, "\n```\n%s\n```\n", toolouts[i].ContentOrToolCall())
				}
			}
		}
		stack = append(stack, toolouts...)
		slog.Info("added toolouts", "n", len(toolouts), "ignored", conf.IsHideToolOuts(), "sent", jsonout != nil, "printed", jsonout == nil)
		continue Chat
	}

	return nil
}
