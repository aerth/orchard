// Package implv1 tool
package implv1

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"m/pkg/chatmessages"
	"m/pkg/llm"
)

type (
	RawCall     = Call[json.RawMessage]
	Call[T any] struct {
		ID       string `json:"id"`
		Function struct {
			Name      string `json:"name"`
			Arguments T      `json:"arguments,omitempty"`
		} `json:"function"`
	}
)

func ReadToolCall[T any](ctx context.Context, caller llm.Message, toolCallIndex int) (Call[T], chatmessages.ChatMessages, error) {
	var call Call[T]
	if err := json.Unmarshal([]byte(caller.ToolCalls[toolCallIndex]), &call); err != nil {
		return call, chatmessages.ChatMessages{
			*chatmessages.New(llm.Message{Role: "tool", Content: "Error parsing args"}),
		}, nil
	}
	return call, nil, nil
}

type fileArgs struct {
	Path string `json:"path"`
}

func ReadFile(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
	// 1. Extract args from the specific tool call in the message history
	// This assumes your Message struct allows access to the raw ToolCalls string slice
	// var args fileArgs
	call, msgs, err := ReadToolCall[fileArgs](ctx, caller, toolCallIndex)
	if err != nil || len(msgs) != 0 {
		return msgs, err
	}
	args := call.Function.Arguments
	data, err := os.ReadFile(args.Path)
	if err != nil {
		return NewResponse(call, fmt.Sprintf("Read error: %v", err)), nil
	}
	x := NewResponse(call, string(data))
	return x, nil
}

func ListDirectory(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
	call, msgs, err := ReadToolCall[fileArgs](ctx, caller, toolCallIndex)
	if err != nil || len(msgs) != 0 {
		return msgs, err
	}
	args := call.Function.Arguments
	entries, err := os.ReadDir(args.Path)
	if err != nil {
		return NewResponse(call, fmt.Sprintf("List error: %v", err)), nil
	}
	var results []string
	for _, e := range entries {
		results = append(results, e.Name())
	}
	x := NewResponse(call, fmt.Sprintf("%v", results))
	return x, nil
}

func NewResponse[T any](call Call[T], resp string) chatmessages.ChatMessages {
	x := chatmessages.ChatMessages{
		*chatmessages.New(llm.Message{
			Role:       "tool",
			Content:    resp,
			ToolCallID: call.ID,
			ToolName:   call.Function.Name,
		}),
	}
	return x
}
