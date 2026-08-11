// Package hgr2 for GatherQuickly implementation (hunter-gatherer)
package hgr2

import (
	"context"
	"fmt"
	"log/slog"
)

var (
	BigChannelSize = 10000
	SmallArraySize = 200
	DEBUG          = false
)

type PlainChannelWrap[T any] struct {
	C <-chan T
}
type ChannelWrapper = PlainChannelWrap[ResponseWrap] // channel with UUID

// ResponseWrap allows the receiver to switch based on V.(type) for handling each packet sent in the channel
type ResponseWrap struct {
	V fmt.Stringer // One of: [[]api.ChatResponse] or [messages.ChatMessage] or error
}

func (c PlainChannelWrap[T]) GatherQuickly(ctx context.Context) (news []T, isDone bool) {
	return GatherQuickly(ctx, c.C)
}

// GatherQuickly is for Update function, and should quickly gather whatever is in the channels buffer.
// Returns isDone true if the channel is closed (meaning the caller should remove and no longer check this genID's chan)
// NOTE: this isDone (agent is complete) is DIFFERENT than an individual pending message's [llm.ChatResponse] Done (meaning that a single message is complete)
func GatherQuickly[T any](ctx context.Context, ch <-chan T) (news []T, isDone bool) {
	var a []T
	for ctx.Err() == nil { // limit for this Update call in the ~2fps loop
		select {
		case <-ctx.Done():
			if len(a) == 0 {
				a = nil
			}
			cause := context.Cause(ctx)
			slog.Error("ctx done", "cause", cause, "err", ctx.Err())
			return a, false
		case newmsg, hasRemaining := <-ch:
			if hasRemaining {
				if a == nil {
					a = make([]T, 0, SmallArraySize)
				}
				a = append(a, newmsg) // we got one
				continue
			}
			if len(a) == 0 {
				a = nil
			}
			slog.Debug("GatherQuickly: gathering is DONE, caller should stop checking", "todo", len(ch), "cap", cap(ch))
			return a, true // channel is closed, caller should stop checking
		default: // prevent blocking
			if len(a) == 0 {
				return nil, false // none since last call
			}
			if len(a) > ((SmallArraySize * 10) / 12) {
				slog.Warn("GatherQuickly: gathered lots in one frame, check faster!", "got", len(a))
			}
			return a, false // some since last call
		}
	}
	return a, false
}
