package hgr

import (
	"context"
	"fmt"
	"log/slog"
	"m/cmdpkg"
	"m/pkg/chat"
	"m/pkg/hgr/hgr2"
	"m/pkg/llm"
	"m/pkg/types/chatmessages"
	"m/pkg/types/otool"
	"strings"

	"github.com/aerth/mostly/stackerr"
	"github.com/google/uuid"
)

// Prefunc simply sends signal that a new message will be streaming to this channel (receiver should append a new blank)
func Prefunc(genID uuid.UUID, threadID uuid.UUID, c chan<- hgr2.ResponseWrap) func(creq *llm.ChatRequest, from string) error {
	callnum := 0
	return func(creq *llm.ChatRequest, from string) error {
		callnum++
		top := creq.Messages[len(creq.Messages)-1]
		almost := *chatmessages.NewFromRole(from, "assistant", "")
		almost.ThreadID = threadID
		slog.Info("Prefunc PREFUNC", "top", top, "callnum", callnum, "message_id", almost.MessageID, "thread", threadID)
		wrapped := hgr2.ResponseWrap{
			V:     almost,
			GenID: genID,
		}
		select {
		case c <- wrapped:
			slog.Info("launched new wrap")
			return nil
		default:
			return fmt.Errorf("could not send new wrapped")
		}
	}
}

// PreFunc newer version
func (h *Handler) PreFunc(creq *llm.ChatRequest, from string) error {
	h.callnum++
	top := creq.Messages[len(creq.Messages)-1]
	almost := *chatmessages.NewFromRole(from, "assistant", "")
	almost.ThreadID = h.ThreadID
	slog.Info("Prefunc PREFUNC", "top", top, "callnum", h.callnum, "message_id", almost.MessageID, "thread", almost.ThreadID)
	wrapped := hgr2.ResponseWrap{
		V:     almost,
		GenID: h.GenID,
	}
	select {
	case h.C <- wrapped:
		slog.Info("launched new wrap")
		return nil
	default:
		return fmt.Errorf("could not send new wrapped")
	}
}

// GeneratorFoo takes elements of e and starts a multi-message round with tool runs
func (e XE) GeneratorFoo(genID uuid.UUID, c chan<- hgr2.ResponseWrap) {
	if e.Context == nil {
		panic("GeneratorFoo was given no ctx")
	}
	if e.Context.Err() != nil {
		slog.Error("context was already cancelled", "err", e.Context.Err(), "cause", context.Cause(e.Context))
		panic("ctx was already cancelled") // should not happen
	}
	e.GenID = genID
	e.ch = c
	if len(e.Handlers) != 0 {
		panic("got handlers")
	}
	e.PreGenerate()
	defer func() {
		e.PostGenerate()
		e.CancelFunc()
	}()
	if e.PostFunc == nil {
		e.PostFunc = Postfunc(genID, c)
	}
	hh := NewChatHandler(genID, c)
	hh.HandleToolOutputs2 = e.NonStreamHandler
	handlers := append(e.Handlers, hh.Handle) // single handler at this point
	ext2 := chat.Ext{
		//	PostFunc: Postfunc(genID, c),
		ShowRequests: e.ShowRequests,
		ThreadID:     e.ThreadID,
		From:         e.From,
		Tools:        e.Tools,
		PostFunc:     e.PostFunc,
		Append:       e.Append,
		Prepend:      e.Prepend,
		PreTools:     e.PreTools,
		Handlers:     handlers,
		AfterGen:     e.AfterGen,

		// Both these utlize the chan
		PreFunc:          hh.PreFunc,           // Prefunc(genID, e.ThreadID, e.From, c), // add new empty assistant message to caller's message stack
		NonStreamHandler: hh.HandleToolOutputs, // add tool outputs (or system messages) to caller's message stack
	}

	e.GenerateFooStage2(genID, c, ext2)
}

func (e2 XE) GenerateFooStage2(genID uuid.UUID, c chan<- hgr2.ResponseWrap, ext2 chat.Ext) {
	var tools otool.Tools
	err := tools.FromMarshalers(ext2.Tools)
	if err != nil {
		fail(genID, stackerr.Errorf("unmarshalers: %w", err), e)
		return
	}
	err = cmdpkg.ChatWith(e.Context, e.ChatMessages, tools, ext2)
	if err != nil {
		fail(genID, stackerr.Errorf("ChatWith failed: %w", err), e)
		return
	}
}

func Postfunc(genID uuid.UUID, c chan<- hgr2.ResponseWrap) func(err error) error {
	if true {
		return nil // ok
	}
	slog.Warn("empty postfunc")
	return func(err error) error {
		return fmt.Errorf("GeneratorFoo postfunc is empty")
	}
}

func fail(genID uuid.UUID, err error, e XE) {
	slog := slog.With("gen_id", genID, "from", e.From, "ntools", len(e.Tools))
	slog.Error("RunChatTools failed", "err", err, "cause", context.Cause(e.Context))
	causemsg := fmt.Sprintf("%+v", err)
	cause := context.Cause(e.Context)
	if cause != nil {
		causemsg = cause.Error()
	}
	if err := e.Send(hgr2.ResponseWrap{
		GenID: genID,
		V: llm.ChatResponse{
			Done:       true,
			DoneReason: stackerr.Errorf("%w", err),
			Message: llm.Message{Role: "system", Content: fmt.Sprintf(
				"%s (%s)",
				chatmessages.CancelMessage, causemsg,
			)},
		},
	}); err != nil {
		slog.Warn("Could not finish up after error", "err", err)
	}
}

func IsFail(content string) bool {
	return strings.HasPrefix(content, chatmessages.CancelMessage)
}
