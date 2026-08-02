package hgr2

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
)

var (
	BigChannelSize = 10000
	SmallArraySize = 200
	DEBUG          = false
)

type channelWrap[T any] struct {
	GenID uuid.UUID
	C     <-chan T
}
type (
	ChannelWrapper = channelWrap[ResponseWrap] // channel with UUID
	ResponseWrap   struct {
		GenID uuid.UUID    // this generator, where multiple generators may operating simultaneously.
		V     fmt.Stringer // One of: [[]api.ChatResponse] or [messages.ChatMessage] or error
		//	T     string    // the type string, for logs etc
	}
)

// GatherQuickly is for Update function, and should quickly gather whatever is in the channels buffer.
// Returns isDone true if the channel is closed (meaning the caller should remove and no longer check this genID's chan)
func (c channelWrap[T]) GatherQuickly(ctx context.Context, ch <-chan T) (news []T, isDone bool) {
	if len(ch) == 0 {
	}
	var a []T
	for ctx.Err() == nil { // limit for this Update call in the ~30fps loop
		select {
		case <-ctx.Done():
			if len(a) == 0 {
				a = nil
			}
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
			slog.Debug("gathering is DONE, caller should stop checking", "todo", len(ch), "cap", cap(ch))
			return a, true // channel is closed, caller should stop checking
		default: // not yet
			if len(a) == 0 {
				return nil, false // none since last call
			}
			if len(a) > ((SmallArraySize * 10) / 12) {
				slog.Warn("gathered lots in one frame, check faster!", "got", len(a))
			}
			return a, false // some since last call
		}
	}
	return a, false
}
