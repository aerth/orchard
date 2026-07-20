package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"m/pkg/chatmessages"
	"m/pkg/debugging"
	"m/pkg/llm"

	"github.com/aerth/mostly/stackerr"
	"github.com/google/uuid"
)

type Ext struct {
	PreFunc func(creq *llm.ChatRequest, pending *chatmessages.ChatMessage) error
	From    string
	Prepend []llm.Message
	Append  []llm.Message
	Tools   []json.Marshaler
	Store   StoreI
}
type StoreI interface {
	LoadThread(context.Context, uuid.UUID, int) (chatmessages.ChatMessages, error)
	Add(context.Context, uuid.UUID, ...*chatmessages.ChatMessage) error
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
	MP  *chatmessages.ChatMessage
	MPS []*chatmessages.ChatMessage
)

func FromDB(ctx context.Context, threadID uuid.UUID, e Ext) (chatmessages.ChatMessages, error) {
	// get messages from db
	if e.Store == nil {
		return nil, fmt.Errorf("no db store to load messages")
	}
	msgs, err := e.Store.LoadThread(ctx, threadID, 10)
	if err != nil {
		return nil, stackerr.Errorf("%w", err)
	}
	return msgs, nil
}

var client *llm.Client

func SetClient(x *llm.Client) {
	client = x
}

func FormatRoles(in []llm.Message) []string {
	a := make([]string, len(in))
	for i := range in {
		a[i] = in[i].Role
	}
	return a
}

func PrintMessages(creq *llm.ChatRequest, pending *chatmessages.ChatMessage) error {
	fmt.Fprintf(os.Stderr, "MESSAGES: %d (%q)\n", len(creq.Messages), FormatRoles(creq.Messages))
	_ = json.NewEncoder(os.Stderr).Encode(creq.Messages)
	return nil
}

// Chat uses 'e' for building a temporary {"messages": []} array
func Chat(ctx context.Context, threadID uuid.UUID, msgs chatmessages.ChatMessages, e Ext) (*chatmessages.ChatMessage, error) {
	if len(e.Tools) == 0 && len(msgs) != 0 {
		return nil, fmt.Errorf("no tools")
	}
	if len(e.Prepend) == 0 && len(msgs) != 0 {
		return nil, stackerr.Errorf("no prepend msgs")
	}
	creq := llm.NewChatRequest()
	creq.Tools = e.Tools
	creq.Messages = append(e.Prepend, msgs.Raw()...)
	if e.Append != nil {
		creq.Messages = append(creq.Messages, e.Append...)
	}
	// make new pending
	pending := chatmessages.NewFromRole(e.From, "assistant", "")
	if e.PreFunc != nil {
		if err := e.PreFunc(creq, pending); err != nil {
			return nil, err
		}
	}
	cl := debugging.NewConsoleLogger()

	// generate
	if client == nil {
		client = llm.New()
	}
	err := client.Chat(ctx, creq, cl.HandleChatResponse, pending.GatherChatResponse)
	if err != nil {
		return pending, stackerr.Errorf("%w", err)
	}
	if e.Store != nil {
		if err := e.Store.Add(context.WithoutCancel(ctx), threadID, pending); err != nil {
			return pending, stackerr.Errorf("%w", err)
		}
	}
	return pending, nil
}
