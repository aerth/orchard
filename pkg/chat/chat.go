package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"uuid"

	"m/orchard/pkg/global"
	"m/orchard/pkg/llm"
	"m/orchard/pkg/types/chatmessages"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/mostly/stackerr"
)

// ChatHandler would be called ChatStreamHandler but that is too long
type (
	ChatHandler     = func(mllm.ChatResponse) error
	ChatHandlerPlus interface {
		Handle(mllm.ChatResponse) error
		HandleToolOutputs(cm chatmessages.ChatMessage)
	}
)

// Ext handles logic for the loop
type Ext struct {
	From     string    // From added to assistant/toolouts
	ThreadID uuid.UUID // will be attached to all messages (generated assistant, tool results, system)

	// PreFunc current streaming output for DISPLAY
	// Called right before llm.Chat call, may be used to cancel
	PreFunc func(creq *mllm.ChatRequest, from string) error `json:"-"`
	// PostFunc should return nil
	PostFunc func(err error) error                 `json:"-"`
	Handlers []mllm.StreamHandler                  `json:"-"` // Handlers for streaming, func (cr ChatResponse) error - see [chatmessages.ChatMessage.GatherChatResponse]
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
		if temp.ThreadID == uuid.Nil() {
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
	Handlers = []mllm.StreamHandler
	Handler  = mllm.StreamHandler
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
var client mllm.E

func SetClient(x mllm.E) {
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

func PrintMessages(creq *mllm.ChatRequest) error {
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

// ChatPlain uses [ChatReq] WITHOUT RUNNING TOOLS uses 'e' for building a temporary {"messages": []} array, using [ChatReq]
// returns a single new message without running tools, only running PreFunc, PostFunc, and all layered stream Handlers in order.
func ChatPlain(ctx context.Context, threadID uuid.UUID, msgs chatmessages.ChatMessages, e Ext) error {
	creq := mllm.Default().NewRequest(append(e.Prepend, msgs.Raw()...), false)
	creq.Tools = e.Tools
	if e.Append != nil {
		creq.Messages = append(creq.Messages, e.Append...)
	}
	return ChatReqPlain(ctx, creq, e)
}

// ChatReqPlain WITHOUT RUNNING TOOLS uses 'e' returns a single new message without running tools, running PreFunc, PostFunc, and all layered stream Handlers in order.
// uses [llm.Client.Chat] using the default global client, creating one from env if first call.
func ChatReqPlain(ctx context.Context, creq *mllm.ChatRequest, e Ext) error {
	if e.PreFunc != nil {
		if err := e.PreFunc(creq, e.From); err != nil {
			return stackerr.Errorf("%w", err)
		}
	}
	if len(e.Handlers) == 0 {
		return fmt.Errorf("no handlers")
	}
	if client.Host.Host == "" {
		client = mllm.Default()
	}
	slog.Info("Chatting", "convo", len(creq.Messages), "tools", len(creq.Tools), "handlers", len(e.Handlers))

	if e.ShowRequests {
		_ = PrintMessages(creq)
	}
	//creq.Messages.TrimThoughts(2)
	client.Handlers = e.Handlers
	//creq.Tools = e.Tools
	//slog.Info("SENDING", "js", len(toJsonIndent(creq)))
	//	global.LogWriter.Write(toJsonIndent(creq))
	creq.Stream = true
	creq.Think.Value = false
	err := client.Do(ctx, *creq, client.Handlers)
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

func toJsonIndent(v any) json.RawMessage {
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		panic(err.Error())
	}
	return b
}

func toJson(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err.Error())
	}
	return b
}
