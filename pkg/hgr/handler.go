package hgr

import (
	"fmt"
	"log/slog"
	"time"

	"m/orchard/pkg/llm"
	"m/orchard/pkg/types/chatmessages"

	"m/orchard/pkg/hgr/hgr2"

	"github.com/google/uuid"
)

// Handler that sends stream ChatResponse and Tool Output messages to a channel
type Handler struct {
	ThreadID           uuid.UUID
	GenID              uuid.UUID
	C                  chan<- hgr2.ResponseWrap
	HandleToolOutputs2 func(cm chatmessages.ChatMessage)
	callnum            int
}

// Send to CHANNEL that is read periodically by calling [hgr2.PlainChannelWrap.GatherQuickly]
func (h *Handler) Send(r hgr2.ResponseWrap) error {
	slog := slog.With("gen_id", h.GenID, "call_num", h.callnum)
	t1 := time.Now()
	for {
		select {
		case <-time.After(time.Second * 3):
			slog.Warn("no sendo after", "d", time.Since(t1))
		case h.C <- r: // send to chan
			if l := len(h.C); l > 200 {
				slog.Info("uhoh, channel receiver is lagging", "chanbuf", l)
			}
			return nil
		default: // could not send
			slog.Error("COULD NOT SEND TO CHANNEL", "str", r.V.String())
			return fmt.Errorf("could not send it")
		}
	}
}

// HandleToolOutputs for tool (or system) role outputs
func (h *Handler) HandleToolOutputs(cm chatmessages.ChatMessage) {
	if cm.ThreadID == uuid.Nil {
		cm.ThreadID = h.ThreadID
	}
	slog := slog.With("fn", "HandleToolOutputs", "msgid", cm.MessageID)
	slog.Info("HANDLE OTHER", "cm", cm.String(), "thr", cm.ThreadID)
	if err := h.Send(hgr2.ResponseWrap{V: cm}); err != nil {
		slog.Error("could not send to chan", "err", err)
	}
	if h.HandleToolOutputs2 != nil {
		h.HandleToolOutputs2(cm)
	}
}

// BeginStream starts a new stream within this channel
func (h *Handler) BeginStream() error {
	cm := chatmessages.NewFromRole("", "assistant", "")
	cm.ThreadID = h.ThreadID
	return h.Send(hgr2.ResponseWrap{
		V: *cm,
	})
}

// Handle implements [chat.Handler]
func (h *Handler) Handle(cr llm.ChatResponse) error {
	return h.Send(hgr2.ResponseWrap{V: cr})
}

// NewChatHandler only sends updates to the channel. Caller may then assign h.HandleToolOutputs2
func NewChatHandler(genid uuid.UUID, ch chan<- hgr2.ResponseWrap, threadID uuid.UUID) *Handler {
	h := &Handler{C: ch, GenID: genid, ThreadID: threadID}
	return h
}
