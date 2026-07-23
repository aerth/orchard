package cmdpkg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"m/cmdpkg/rege"
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
)

type EncoderFunc = func(v chatmessages.ChatMessage) error

var JsonOut EncoderFunc

func ChatWith(ctx context.Context, stack chatmessages.ChatMessages, newmsg *chatmessages.ChatMessage, tools otool.Tools, e chat.Ext) error {
	showChar := false
	conf := global.Get()
	if conf.GetVerbose() {
		e.PreFunc = chat.PrintMessages
	}
	if newmsg != nil && IsCommand(newmsg.Content) {
		add, err := HandleCommand(newmsg, conf, &e, &stack)
		// dont have caller generate
		cmdmsg := *newmsg
		newmsg = nil
		if cmdmsg.Content != "/clear" {
			stack = append(stack, cmdmsg)
		}
		stack = append(stack, add...)
		for _, v := range add {
			_, _ = fmt.Fprintf(os.Stdout, "%s> %s\n", v.Role, v.ContentOrToolCall())
		}
		if err != nil {
			return stackerr.Errorf("HandleCommand: %w", err)
		}
		if add.TopRole("user").Content == "" {
			return nil
		}
	}
	if newmsg == nil || newmsg.Content == "" {
		return nil
	}

	if ss := rege.FindImagePaths(newmsg.Content); len(ss) != 0 {
		newmsg.Attachments = ss
		for _, filename := range ss {
			img, err := os.ReadFile(filename)
			if err != nil {
				slog.Error("attaching file", "err", err, "filename", filename)
				err = stackerr.Errorf("Error attaching %q: %w", filename, err)
				newmsg.Content += "\n" + err.Error()
				continue
			}
			newmsg.Images = append(newmsg.Images, img)
		}
		slog.Info("Attached", "names", ss)
	} else {
		slog.Info("no attachments found in message", "txt", newmsg.Content)
	}
	if e.Store != nil {
		if err := e.Store.Add(ctx, conf.GetThreadID(), newmsg); err != nil {
			return stackerr.Errorf("%w", err)
		}
	}
	stack = append(stack, *newmsg)
	var out io.Writer = os.Stdout
	var jsonout EncoderFunc
	if JsonOut != nil {
		jsonout = JsonOut
	}
	if conf.Get().JsonOutput {
		out = io.Discard
		enc := json.NewEncoder(os.Stdout).Encode
		jsonout = func(v chatmessages.ChatMessage) error {
			return enc(v)
		}
	}
	cl := debugging.NewConsoleLogger(out, os.Stderr)
	cl.HideToolCalls = conf.IsHideToolCalls()
	var err error
Chat:
	e.Prepend = []llm.Message{{Role: "system", Content: global.GetSystemBase(templatedata.NewData(stack))}}
	sctx := global.NewCancel(ctx)
	e.Tools = tools.ToMarshaler()
	cl.Reset()
	got := chatmessages.NewFromRole(e.From, "assistant", "")
	slog.Info("Chatting", "after", stack.Top())
	spin := spinner.Start(sctx)
	e.Handlers = []llm.ChatStreamHandlerFunc{spin.HandleChatResponse, cl.HandleChatResponse, got.GatherChatResponse}
	_, err = chat.Chat(sctx, conf.GetThreadID(), stack, e)
	_ = spin.HandleChatResponse(llm.ChatResponse{})
	if err != nil && !errors.Is(err, context.Canceled) {
		return stackerr.Errorf("%w", err)
	}

	if errors.Is(err, context.Canceled) {
		got.Content += " (this reply was cancelled by user before it finished)"
	}
	if e.Store != nil {
		if err := e.Store.Add(context.WithoutCancel(ctx), conf.GetThreadID(), got); err != nil {
			return stackerr.Errorf("%w", err)
		}
	}
	stack = append(stack, *got)
	if jsonout != nil {
		if err := jsonout(*got); err != nil {
			return stackerr.Errorf("%w", err)
		}
	}

	/*
		// show output
		if showChar {
			fmt.Fprintf(os.Stderr, "\nBOT> ")
		}
		w := os.Stdout
		if len(got.ToolCalls) != 0 {
			w = os.Stderr
		}
		_, _ = fmt.Fprintf(w, "%s\n", got.ContentOrToolCall())
	*/
	// handle tools
	if len(got.ToolCalls) != 0 {
		toolouts, err := toolrunner.Runtools(ctx, tools, *got)
		if WrapToolErrors && err != nil {
			toolouts = append(toolouts, WrapToolError(err)...)
			err = nil
		}
		if len(toolouts) == 0 {
			toolouts = append(toolouts, WrapToolError(fmt.Errorf("no output"))...)
		}
		if e.Store != nil {
			if err := e.Store.Add(ctx, conf.GetThreadID(), chat.Ptrs(toolouts)...); err != nil {
				return stackerr.Errorf("adding tool output to db: %w", err)
			}
		}
		if !conf.IsHideToolOuts() {
			if jsonout != nil {
				for i := range toolouts {
					if err := jsonout(toolouts[i]); err != nil {
						return stackerr.Errorf("%w", err)
					}
				}
			}
			if jsonout == nil {
				for i := range toolouts {
					if showChar {
						_, _ = fmt.Fprintf(os.Stderr, "TOOL> ")
					}
					_, _ = fmt.Fprintf(os.Stderr, "\n```\n%s\n```\n", toolouts[i].ContentOrToolCall())
				}
			}
		}
		if err != nil {
			return stackerr.Errorf("running tools: %w", err)
		}
		stack = append(stack, toolouts...)
		goto Chat
	}

	return nil
}
