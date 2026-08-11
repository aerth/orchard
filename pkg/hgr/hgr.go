// Package hgr chat hunter gatherer
package hgr

import (
	"context"
	"fmt"
	"log/slog"

	"m/pkg/chat"
	"m/pkg/hgr/hgr2"
	"m/pkg/types/chatmessages"

	"github.com/google/uuid"
)

// Starter to add a channel for tracking
type Starter hgr2.ChannelWrapper

// Refresher may be sent on the channel
type Refresher struct {
	GenID      uuid.UUID
	IsDone     bool // ONLY for main model to focus (unblur) the text input area.
	DoneReason any
}

// NewG runs generatorFN in a goroutine, then closes channel and sends Finisher.
// NOTE: generatorFn should not close channel
func NewG[T any](genID uuid.UUID, size int, generatorFn func(uuid.UUID, chan<- T)) <-chan T {
	if generatorFn == nil {
		panic("no generatorFn")
	}
	ch := make(chan T, size)
	go func() {
		generatorFn(genID, ch)
		close(ch) //     channel is closed, THEN finisher is sent. but channel is buffered and may still contain cr to gather.
	}()
	return ch
}

type XE struct { // immutable. sends api.Response to channel
	chat.Ext
	GenID uuid.UUID
	// otool.Tools
	Context context.Context
	chatmessages.ChatMessages
	Loader             func(from string) chatmessages.ChatMessages
	context.CancelFunc        // called always once after PostGenerate
	PreGenerate        func() // called always once immediately when generator starts
	PostGenerate       func() // called always once after generate
	ch                 chan<- hgr2.ResponseWrap
}

func (e XE) Encoder(msgptr *chatmessages.ChatMessage) error {
	msg := *msgptr
	if msg.ThreadID == uuid.Nil {
		msg.ThreadID = e.ThreadID
	}
	if false && msg.Role == "assistant" {
		slog.Warn("encoder: skipping assistant", "m", msg.String())
		return nil
	}
	return e.Send(hgr2.ResponseWrap{
		V: msg,
	})
}

func (e XE) Send(r hgr2.ResponseWrap) error {
	select {
	case e.ch <- r:
		return nil
	default:
		return fmt.Errorf("could not send update uuid=%s val=%T %v", r.GenID, r.V, r.V)
	}
}

// Defaults may only set three: PreGenerate PostGenerate CancelFunc, or use others to store global Defaults (such as Loader)
var Defaults = XE{PreGenerate: func() {}, PostGenerate: func() {}, CancelFunc: func() {}}

// New sends a HereIsChannel to read.
// The 'e' Ext MUST contain 'From' and caller MUST empty the HereIsChannel which is automatically closed when generation completes.
func New(ctx context.Context, id uuid.UUID, stack chatmessages.ChatMessages, e chat.Ext, loader func(name string) chatmessages.ChatMessages) (<-chan hgr2.ResponseWrap, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if e.From == "" {
		panic("wtf")
	}
	ch := StartNewChan(ctx, id, stack, e, loader)
	return ch, nil
}

func StartNewChan(ctx context.Context, genID uuid.UUID, stack chatmessages.ChatMessages, e chat.Ext, loader func(name string) chatmessages.ChatMessages) <-chan hgr2.ResponseWrap {
	if stack.Top().Role == "assistant" {
		panic("is assistant")
	}
	if e.From == "" {
		panic("wtf")
	}
	xe := XE{
		Context: ctx, Ext: e, ChatMessages: stack, Loader: loader,
		PostGenerate: Defaults.PostGenerate,
		CancelFunc:   Defaults.CancelFunc,
		PreGenerate:  Defaults.PreGenerate,
	}
	ch := NewG(genID, 10000, xe.GeneratorFoo)
	return ch // if Sender is nil, caller is expected to consume channel. for tests
}
