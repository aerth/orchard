package hgr

import (
	"fmt"
	"log/slog"
	"m/pkg/chatmessages"
	"m/pkg/llm"
	"time"

	"oc/pkg/hgr/hgr2"

	"github.com/google/uuid"
)

// Handler that sends stream ChatResponse and Tool Output messages to a channel
type Handler struct {
	GenID              uuid.UUID
	C                  chan<- hgr2.ResponseWrap
	HandleToolOutputs2 func(cm chatmessages.ChatMessage)
}

// Send to CHANNEL
func (h *Handler) Send(r hgr2.ResponseWrap) error {
	t1 := time.Now()
	if r.GenID == uuid.Nil {
		r.GenID = h.GenID
	}
	for {
		select {
		case <-time.After(time.Second * 3):
			slog.Warn("no sendo after", "d", time.Since(t1))
		case h.C <- r:
			if l := len(h.C); l > 40 {
				slog.Info("uhoh", "chanbuf", l)
			}
			return nil
		default:
			slog.Error("COULD NOT SEND TO CHANNEL", "str", r.V.String())
			return fmt.Errorf("could not send it")
		}
	}
}

// HandleToolOutputs for tool (or system) role outputs
func (h *Handler) HandleToolOutputs(cm chatmessages.ChatMessage) {
	slog := slog.With("fn", "HandleToolOutputs", "msgid", cm.MessageID)
	slog.Info("HANDLE OTHER")
	if err := h.Send(hgr2.ResponseWrap{V: cm}); err != nil {
		slog.Error("could not send to chan", "err", err)
	}
	if h.HandleToolOutputs2 != nil {
		h.HandleToolOutputs2(cm)
	}
}

// BeginStream starts a new stream within this channel
func (h *Handler) BeginStream() error {
	return h.Send(hgr2.ResponseWrap{V: *chatmessages.NewFromRole("", "assistant", "")})
}

// Handle implements [chat.Handler]
func (h *Handler) Handle(cr llm.ChatResponse) error {
	return h.Send(hgr2.ResponseWrap{V: cr})
}

// NewChatHandler only sends updates to the channel. Caller may then assign h.HandleToolOutputs2
func NewChatHandler(genid uuid.UUID, ch chan<- hgr2.ResponseWrap) *Handler {
	h := &Handler{C: ch, GenID: genid}
	return h
}
