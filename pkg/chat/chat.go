package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"m/pkg/global"
	"m/pkg/llm"
	"m/pkg/types/chatmessages"
	"slices"

	"github.com/aerth/mostly/stackerr"
	"github.com/google/uuid"
)

// ChatHandler would be called ChatStreamHandler but that is too long
type (
	ChatHandler     = func(llm.ChatResponse) error
	ChatHandlerPlus interface {
		Handle(llm.ChatResponse) error
		HandleToolOutputs(cm chatmessages.ChatMessage)
	}
)

// Ext handles logic for the loop
type Ext struct {
	From     string    // From added to assistant/toolouts
	ThreadID uuid.UUID // will be attached to all messages (generated assistant, tool results, system)

	// PreFunc current streaming output for DISPLAY
	// Called right before llm.Chat call, may be used to cancel
	PreFunc func(creq *llm.ChatRequest, from string) error `json:"-"`
	// PostFunc should return nil
	PostFunc func(err error) error                 `json:"-"`
	Handlers []ChatHandler                         `json:"-"` // Handlers for streaming, func (cr ChatResponse) error - see [chatmessages.ChatMessage.GatherChatResponse]
	AfterGen func(*chatmessages.ChatMessage) error `json:"-"` // after generation is complete (assistant, toolcalls) for applying modifications or exit early or store to DB
	// NonStreamHandler for any toolouts or temporary system messages for DISPLAY
	// for tool/system (not generated assistant stream)
	NonStreamHandler func(chatmessages.ChatMessage) `json:"-"`

	PreTools func(ctx context.Context, newcaller *chatmessages.ChatMessage, functionNames []string) error `json:"-"` // for example, a confirmation, or whitelist

	Prepend []llm.Message    // prepended to messages (tmp)
	Append  []llm.Message    // appended to messages (tmp)
	Tools   []json.Marshaler // Tool specs available
	//
	ShowRequests bool            // from conf
	H            ChatHandlerPlus `json:"-"`
}

// FromA (internal) combines variadic Exts into one
func (e Ext) FromA(exts ...Ext) Ext {
	temp := e
	for i := range exts {

		// appenders
		temp.Handlers = append(temp.Handlers, exts[i].Handlers...)
		temp.Append = append(temp.Append, exts[i].Append...)
		temp.Prepend = append(temp.Prepend, exts[i].Prepend...)
		temp.Tools = append(temp.Tools, exts[i].Tools...)

		// replacers
		if exts[i].From != "" {
			temp.From = exts[i].From
		}
		if temp.ThreadID == uuid.Nil {
			temp.ThreadID = exts[i].ThreadID
		}
		if exts[i].PreTools != nil {
			temp.PreTools = exts[i].PreTools
		}
		if exts[i].AfterGen != nil {
			temp.AfterGen = exts[i].AfterGen
		}
		if exts[i].PreFunc != nil {
			temp.PreFunc = exts[i].PreFunc
		}
		if exts[i].PostFunc != nil {
			temp.PostFunc = exts[i].PostFunc
		}
		if exts[i].NonStreamHandler != nil {
			temp.NonStreamHandler = exts[i].NonStreamHandler
		}
		temp.ShowRequests = temp.ShowRequests || exts[i].ShowRequests
		if exts[i].H != nil {
			temp.H = exts[i].H
		}
	}
	return temp
}

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
	w := global.LogWriter
	slog.Info("showing messages", "n", len(creq.Messages), "roles", FormatRoles(creq.Messages))
	_, _ = fmt.Fprintf(w, "MESSAGES: %d (%q)\n", len(creq.Messages), FormatRoles(creq.Messages))
	msg := slices.Clone(creq.Messages)
	if DebugTrimImages {
		for i := range msg {
			if len(msg[i].Images) != 0 {
				msg[i].Images = nil
			}
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	enc.SetEscapeHTML(true)
	_ = enc.Encode(msg)
	return nil
}

// Chat WITHOUT RUNNING TOOLS uses 'e' for building a temporary {"messages": []} array, using [ChatReq]
// returns a single new message without running tools, only running PreFunc, PostFunc, and all layered stream Handlers in order.
func Chat(ctx context.Context, threadID uuid.UUID, msgs chatmessages.ChatMessages, e Ext) error {
	creq := llm.NewChatRequest()
	creq.Tools = e.Tools
	creq.Messages = append(e.Prepend, msgs.Raw()...)
	if e.Append != nil {
		creq.Messages = append(creq.Messages, e.Append...)
	}
	return ChatReq(ctx, creq, e)
}

// ChatReq WITHOUT RUNNING TOOLS uses 'e' returns a single new message without running tools, running PreFunc, PostFunc, and all layered stream Handlers in order.
// uses [llm.Client.Chat] using the default global client, creating one from env if first call.
func ChatReq(ctx context.Context, creq *llm.ChatRequest, e Ext) error {
	if e.PreFunc != nil {
		if err := e.PreFunc(creq, e.From); err != nil {
			return err
		}
	}
	if client == nil {
		client = llm.New()
	}
	if len(e.Handlers) == 0 {
		return fmt.Errorf("no handlers")
	}
	slog.Info("Chatting", "convo", len(creq.Messages), "tools", len(creq.Tools), "handlers", len(e.Handlers))
	if e.ShowRequests {
		_ = PrintMessages(creq)
	}

	creq.Messages.TrimThoughts(2)
	err := client.Chat(ctx, creq, e.Handlers...)
	if err != nil {
		slog.Error("Chatting error", "err", err, "cause", context.Cause(ctx))
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
