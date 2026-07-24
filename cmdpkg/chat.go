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

type EncoderFunc = func(v *chatmessages.ChatMessage) error

var MasterSend EncoderFunc = nil // = func(v *chatmessages.ChatMessage) error {
//	return nil
//}

func ChatWith(ctx context.Context, stackInput chatmessages.ChatMessages, tools otool.Tools, e chat.Ext) error {
	stack := stackInput
	showChar := false
	conf := global.Get()
	if conf.GetVerbose() {
		e.PreFunc = chat.PrintMessages
	}
	if len(stack) == 0 {
		slog.Warn("noop convo")
		return nil
	}
	newmsg := &stack[len(stack)-1]
	if newmsg.Role != "user" {
		return fmt.Errorf("expected 'user' role to generate a reply to, but got '%s'", newmsg.Role)
	}
	if IsCommand(newmsg.Content) {
		slog.Info("detected command", "txt", newmsg.ContentOrToolCall())
		add, err := HandleCommand(newmsg, &e, &stack)
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
		slog.Info("noop txt")
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
	if MasterSend != nil {
		out = io.Discard
		jsonout = MasterSend
	} else if conf.Get().JsonOutput {
		out = io.Discard
		enc := json.NewEncoder(os.Stdout).Encode
		jsonout = func(v *chatmessages.ChatMessage) error {
			return enc(v)
		}
	}
	cl := debugging.NewConsoleLogger(out, global.LogWriter)
	cl.HideToolCalls = conf.IsHideToolCalls()
	var err error
Chat:
	for ctx.Err() == nil {
		e.Prepend = []llm.Message{{Role: "system", Content: global.GetSystemBase(templatedata.NewData(stack))}}
		sctx := global.NewCancel(ctx)
		e.Tools = tools.ToMarshaler()
		cl.Reset()
		got := chatmessages.NewFromRole(e.From, "assistant", "")
		top := stack.Top()
		if top.Role == "assistant" {
			slog.Error("already is assistant, cant generate", "top", top.String(), "roles", stack.FormatRoles())
			return nil
		}
		slog.Info("Chatting", "after", top)

		{
			spin := spinner.Start(sctx)
			e.Handlers = []llm.ChatStreamHandlerFunc{spin.HandleChatResponse, cl.HandleChatResponse, got.GatherChatResponse}
			err = chat.Chat(sctx, conf.GetThreadID(), stack, e)
			_ = spin.HandleChatResponse(llm.ChatResponse{}) // ensure stopped
		}
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
			// jsonout encoder may modify the pointer
			if err := jsonout(&stack[len(stack)-1]); err != nil {
				return stackerr.Errorf("%w", err)
			}
		}
		// handle tools
		if len(got.ToolCalls) == 0 {
			return nil
		}
		slog.Info("detected tool calls", "ncalls", len(got.ToolCalls))
		toolouts, err := toolrunner.Runtools(ctx, tools, *got)
		slog.Info("finished running tool calls", "ncalls", len(got.ToolCalls), "err", err, "nresp", len(toolouts))
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
		if MasterSend != nil || (!conf.IsHideToolOuts() && jsonout != nil) { // if external handled, send all
			for i := range toolouts {
				if err := jsonout(&toolouts[i]); err != nil {
					return stackerr.Errorf("%w", err)
				}
			}
		}
		if !conf.IsHideToolOuts() {
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
		slog.Info("added toolouts", "n", len(toolouts), "ignored", conf.IsHideToolOuts(), "sent", jsonout != nil, "printed", jsonout == nil)
		continue Chat
	}

	return nil
}
