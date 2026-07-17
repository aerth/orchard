package chatmessages

import (
	"context"
	"encoding/json"
	"fmt"

	"m/pkg/debugging"
	"m/pkg/llm"

	"github.com/aerth/mostly/stackerr"
	"github.com/google/uuid"
)

type Ext struct {
	Prepend []llm.Message
	Append  []llm.Message
	Tools   []json.Marshaler
	Store   StoreI
}
type StoreI interface {
	LoadThread(context.Context, uuid.UUID, int) (ChatMessages, error)
	Add(context.Context, uuid.UUID, ...*ChatMessage) error
}

// Ptrs tries not to copy
func Ptrs[T any, A ~[]T](o A) []*T {
	a := make([]*T, len(o))
	for i := range o {
		a[i] = &o[i]
	}
	return a
}

// Vals leaves nil items as zero-value
func Vals[T any, A ~[]*T](o A) []T {
	a := make([]T, len(o))
	for i := range o {
		if o[i] == nil {
			continue
		}
		a[i] = *o[i]
	}
	return a
}

type (
	MP  *ChatMessage
	MPS []*ChatMessage
)

func ChatFromDB(ctx context.Context, threadID uuid.UUID, newmsg ChatMessage, e Ext) (*ChatMessage, error) {
	// get messages from db
	if e.Store == nil {
		return nil, fmt.Errorf("no db store to load messages")
	}
	msgs, err := e.Store.LoadThread(ctx, threadID, 100)
	if err != nil {
		return nil, stackerr.Errorf("%w", err)
	}
	msgs = append(msgs, newmsg)
	return Chat(ctx, threadID, msgs, e)
}

var client *llm.Client

func SetClient(x *llm.Client) {
	client = x
}

func Chat(ctx context.Context, threadID uuid.UUID, msgs ChatMessages, e Ext) (*ChatMessage, error) {
	creq := llm.NewChatRequest()
	creq.Messages = append(e.Prepend, msgs.RemoveThoughts().Raw()...)
	creq.Tools = e.Tools
	if e.Append != nil {
		creq.Messages = append(creq.Messages, e.Append...)
	}

	// make new pending
	pending := New(llm.Message{})
	cl := debugging.NewConsoleLogger()

	// generate
	if client == nil {
		client = llm.New()
	}
	err := client.Chat(ctx, creq, cl.HandleChatResponse, pending.GatherChatResponse)
	if err != nil {
		return pending, stackerr.Errorf("%w", err)
	}
	if err := e.Store.Add(ctx, threadID, pending); err != nil {
		return pending, stackerr.Errorf("%w", err)
	}
	return pending, nil
}
