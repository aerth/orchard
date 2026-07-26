package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"

	"m/pkg/chatmessages"
	"m/pkg/llm"

	"github.com/aerth/mostly/stackerr"
	"github.com/google/uuid"
)

type (
	ChatHandler = func(llm.ChatResponse) error
	Ext         struct {
		ThreadID uuid.UUID                         // will be attached to all messages (generated assistant, tool results, system)
		PreFunc  func(creq *llm.ChatRequest) error // Called right before llm.Chat call, may be used to cancel
		PostFunc func(err error) error             // should return nil
		Handlers []ChatHandler                     // Handlers for streaming, func (cr ChatResponse) error - see [chatmessages.ChatMessage.GatherChatResponse]

		AfterGen         func(*chatmessages.ChatMessage) error // for toolouts/system (not generated assistant stream)
		NonStreamHandler func(chatmessages.ChatMessage)        // for toolouts/system (not generated assistant stream)
		From             string                                // From added to assistant/toolouts
		Prepend          []llm.Message                         // prepended to messages (tmp)
		Append           []llm.Message                         // appended to messages (tmp)
		Tools            []json.Marshaler                      // Tool specs available
	}
)

type (
	Handlers = []ChatHandler
	Handler  = ChatHandler
)

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

type IsToolArgument interface{ IsToolArgument() }

// ReadToolCall may return an error message
func ReadToolCall[T IsToolArgument](ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.Call[T], error) {
	var call chatmessages.Call[T]
	if err := json.Unmarshal([]byte(caller.ToolCalls[toolCallIndex]), &call); err != nil {
		return call, stackerr.Errorf("unmarshal json arguments into call[%T]: %w", call.Function.Arguments, err)
	}
	if call.Function.Name == "" {
		return call, stackerr.Errorf("could not decode json arguments into call[%T]", call.Function.Arguments)
	}
	return call, nil
}

func NewErrorMessage(role string, content string) chatmessages.ChatMessages {
	return chatmessages.ChatMessages{
		*chatmessages.New(llm.Message{Role: "tool", Content: "Error parsing args"}),
	}
}

type (
	MP  *chatmessages.ChatMessage
	MPS []*chatmessages.ChatMessage
)

/*
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
*/
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

var DebugTrimImages = true

func PrintMessages(creq *llm.ChatRequest) error {
	slog.Info("showing messages", "n", len(creq.Messages), "roles", FormatRoles(creq.Messages))
	fmt.Fprintf(os.Stderr, "MESSAGES: %d (%q)\n", len(creq.Messages), FormatRoles(creq.Messages))
	msg := slices.Clone(creq.Messages)
	if DebugTrimImages {
		for i := range msg {
			if len(msg[i].Images) != 0 {
				msg[i].Images = nil
			}
		}
	}

	enc := json.NewEncoder(os.Stderr)
	enc.SetIndent("", " ")
	enc.SetEscapeHTML(true)
	_ = enc.Encode(msg)
	return nil
}

// Chat uses 'e' for building a temporary {"messages": []} array, returning a single new message
func Chat(ctx context.Context, threadID uuid.UUID, msgs chatmessages.ChatMessages, e Ext) error {
	if len(e.Tools) == 0 && len(msgs) != 0 {
		return fmt.Errorf("no tools")
	}
	if len(e.Prepend) == 0 && len(msgs) != 0 {
		return stackerr.Errorf("no prepend msgs")
	}
	creq := llm.NewChatRequest()
	creq.Tools = e.Tools
	creq.Messages = append(e.Prepend, msgs.Raw()...)
	if e.Append != nil {
		creq.Messages = append(creq.Messages, e.Append...)
	}
	return ChatReq(ctx, creq, e)
}

func ChatReq(ctx context.Context, creq *llm.ChatRequest, e Ext) error {
	if e.PreFunc != nil {
		slog.Info("runnign prefunc")
		if err := e.PreFunc(creq); err != nil {
			return err
		}
	}
	//var pending *chatmessages.ChatMessage
	// generate
	if client == nil {
		client = llm.New()
	}
	if len(e.Handlers) == 0 {
		return fmt.Errorf("no handlers")
	}
	slog.Info("Chatting", "convo", len(creq.Messages), "tools", len(creq.Tools), "handlers", len(e.Handlers))
	err := client.Chat(ctx, creq, e.Handlers...)
	if err != nil {
		slog.Error("Chatting error", "err", err)
	}
	if e.PostFunc != nil {
		if e := e.PostFunc(err); e != nil {
			err = errors.Join(err, stackerr.Errorf("%w", e))
		}
	}
	if err != nil {
		return stackerr.Errorf("%w", err)
	}
	return nil
}

// FromA (internal) combines variadic Exts into one
func (e Ext) FromA(exts ...Ext) Ext {
	temp := e
	for i := range exts {
		// appenders
		temp.Handlers = append(temp.Handlers, exts[i].Handlers...)
		temp.Append = append(temp.Append, exts[i].Append...)
		temp.Prepend = append(temp.Prepend, exts[i].Prepend...)
		// replacers
		if exts[i].From != "" {
			temp.From = exts[i].From
		}
	}
	return temp
}
