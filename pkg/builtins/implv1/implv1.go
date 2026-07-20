// Package implv1 tool
package implv1

import (
	"context"
	"fmt"
	"os"

	"m/pkg/chatmessages"
	"m/pkg/llm"
)

type fileArgs struct {
	Path string `json:"path"`
}
type ToolArg struct{}

func (ToolArg) IsToolArgument()    {}
func (f fileArgs) IsToolArgument() {}

func ReadFile(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
	// 1. Extract args from the specific tool call in the message history
	// This assumes your Message struct allows access to the raw ToolCalls string slice
	// var args fileArgs
	call, err := chatmessages.ReadToolCall[fileArgs](ctx, caller, toolCallIndex)
	if err != nil {
		msg := *chatmessages.New(llm.Message{Role: "tool", Content: fmt.Sprintf("Error parsing args: %+v", err)})
		return chatmessages.ChatMessages{msg}, nil
	}
	args := call.Function.Arguments
	data, err := os.ReadFile(args.Path)
	if err != nil {
		return NewResponse(call, fmt.Sprintf("ReadFile error: %v", err)), nil
	}
	x := NewResponse(call, string(data))
	return x, nil
}

func ListDirectory(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
	call, err := chatmessages.ReadToolCall[fileArgs](ctx, caller, toolCallIndex)
	if err != nil {
		msg := *chatmessages.New(llm.Message{Role: "tool", Content: fmt.Sprintf("Error parsing args: %+v", err)})
		return chatmessages.ChatMessages{msg}, nil
	}
	args := call.Function.Arguments
	entries, err := os.ReadDir(args.Path)
	if err != nil {
		return NewResponse(call, fmt.Sprintf("ListDirectory error: %v", err)), nil
	}
	var results []string
	for _, e := range entries {
		results = append(results, e.Name())
	}
	x := NewResponse(call, fmt.Sprintf("%v", results))
	return x, nil
}

func NewResponse[T any](call chatmessages.Call[T], resp string) chatmessages.ChatMessages {
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
