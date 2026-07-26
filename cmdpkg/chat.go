package cmdpkg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
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

var MasterSend EncoderFunc = nil // = func(v *chatmessages.ChatMessage) error {
//	return nil
//}

func ChatWith(ctx context.Context, stackInput chatmessages.ChatMessages, tools otool.Tools, e chat.Ext) error {
	stack := stackInput
	showChar := false
	conf := global.Get()
	if e.PreFunc == nil && conf.GetVerbose() {
		e.PreFunc = chat.PrintMessages
	}
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
			out = io.Discard
			tw = io.Discard
			jsonout = MasterSend
		} else if conf.Get().JsonOutput {
			out = io.Discard
			tw = io.Discard
			enc := json.NewEncoder(os.Stdout).Encode
			jsonout = func(v *chatmessages.ChatMessage) error {
				return enc(v)
			}
		}
		cl = debugging.NewConsoleLogger(out, tw)
	}
	var err error
	handlers := e.Handlers
Chat:
	for ctx.Err() == nil {
		for i := range stack {
			if stack[i].Role != "system" && stack[i].MessageID == uuid.Nil || stack[i].ThreadID == uuid.Nil {
				return fmt.Errorf("detected a non-system (%s) message with no IDs: %d: %s", stack[i].Role, i, stack[i].String())
			}
		}
		e.Prepend = []llm.Message{{Role: "system", Content: global.GetSystemBase(templatedata.NewData(stack))}}
		sctx := global.NewCancel(ctx)
		e.Tools = tools.ToMarshaler()
		if cl != nil {
			cl.Reset()
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
		if false {
			newcaller = nil
		}
		{
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
			err = chat.Chat(sctx, conf.GetThreadID(), stack, e)
			if spin != nil {
				_ = spin.HandleChatResponse(llm.ChatResponse{}) // ensure spinner is stopped first regardless of output
			}
			if summary, ok := newcaller.Summary.(string); ok {
				global.LogWriter.Write([]byte("\n" + summary + "\n"))
			} else {
				global.LogWriter.Write([]byte("\n" + "NO SUMMARY FOUND" + "\n"))
			}
		}
		newcaller.GenDuration = time.Since(newcaller.CreatedAt)
		if err != nil && !errors.Is(err, context.Canceled) {
			return stackerr.Errorf("gen: %w", err)
		}
		if errors.Is(err, context.Canceled) {
			newcaller.Content += " (this reply was cancelled by user before it finished)"
		}
		stack = append(stack, *newcaller)
		newcaller = &stack[len(stack)-1] // fix ptr
		/*
			if e.Store != nil {
				// Add sets initial msg.DBID
				if err := e.Store.Add(context.WithoutCancel(ctx), e.ThreadID, newgenerated); err != nil {
					return stackerr.Errorf("%w", err)
				}
			}*/
		if jsonout != nil {
			// jsonout encoder may modify the pointer (TODO: hook for confirmation/cancel)
			if err := jsonout(&stack[len(stack)-1]); err != nil {
				return stackerr.Errorf("%w", err)
			}
		}
		// handle tools
		if len(newcaller.ToolCalls) == 0 {
			return nil
		}
		//
		slog.Info("detected tool calls", "ncalls", len(newcaller.ToolCalls))
		// run the tool exec funcs
		toolouts, err := toolrunner.Runtools(ctx, tools, *newcaller)
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
			if MasterSend != nil || (!conf.IsHideToolOuts() && jsonout != nil) { // if external handled, send all
				for i := range toolouts {
					if err := jsonout(&toolouts[i]); err != nil {
						return stackerr.Errorf("%w", err)
					}
				}
			}
			if e.NonStreamHandler != nil {
				for i := range toolouts {
					e.NonStreamHandler(toolouts[i])
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
