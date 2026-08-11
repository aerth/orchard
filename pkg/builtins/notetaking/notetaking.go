// Package notetaking provides the take_notes interface Tool.
package notetaking

import (
	"context"
	"log/slog"
	"time"

	"m/cmdpkg"
	"m/pkg/chat"
	"m/pkg/llm"
	"m/pkg/toolrunner"
	"m/pkg/types/chatmessages"
	"m/pkg/types/otool"

	"github.com/aerth/mostly/stackerr"
)

type Note struct {
	Text string
	Time time.Time
}

const OpEnd = "END"

var prompt = "You are now in a notetaking session. To stop taking notes, output only END on a single line. You can use tools, and output notes directly as content. All output will be saved for reviewing after done with notetaking."

var NoteToolName = "take_notes"

type args struct {
	Intent string `json:"intent"` // The intention for starting the notetaking session
}

func (a args) IsToolArgument() {}

func ExecFunc(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
	ext, ok := ctx.Value(toolrunner.CtxExt).(chat.Ext)
	if !ok {
		return nil, stackerr.Errorf("could not find Ext in ctx")
	}
	tools, ok := ctx.Value(toolrunner.CtxTools).([]otool.Tool)
	if !ok {
		return nil, stackerr.Errorf("could not find Ext in ctx")
	}
	call, err := chat.ReadToolCall[args](ctx, caller, toolCallIndex)
	if err != nil {
		return nil, stackerr.Errorf("parsing call: %w", err)
	}
	_ = call // get intent
	handlers := ext.Handlers
	var convo chatmessages.ChatMessages
	convo = append(convo, *chatmessages.NewThread(ext.ThreadID, llm.Message{Role: "user", Content: "Begin taking notes. Intention: " + call.Function.Arguments.Intent}))
	{
		first := chatmessages.NewThread(ext.ThreadID, llm.Message{Role: "system", Content: prompt})
		first.Ephemeral = true
		convo = append(convo.RemoveEphemeral(), *first)
	}
	for ctx.Err() == nil {
		// gen
		newmsg := chatmessages.NewThread(ext.ThreadID, llm.Message{Role: "assistant"}) // generate now
		convo = append(convo, *newmsg)
		newmsg = &convo[len(convo)-1]
		ext.Handlers = append(handlers, newmsg.GatherChatResponse)
		err := cmdpkg.ChatWith(ctx, convo, tools, ext)
		if err != nil {
			slog.Error("ouch", "err", err)
			return convo, err
		}

		// done gen
		statmsg := chatmessages.NewThread(ext.ThreadID, llm.Message{Role: "system"})
		statmsg.Content = "Note has been recorded. Proceed. " + prompt
		statmsg.Ephemeral = true
		convo = append(convo.RemoveEphemeral(), *statmsg)

	}
	return convo, nil
}
