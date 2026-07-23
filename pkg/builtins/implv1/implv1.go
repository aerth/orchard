// Package implv1 tool
package implv1

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"m/pkg/chat"
	"m/pkg/chatmessages"
	"m/pkg/global"
	"m/pkg/llm"

	"github.com/aerth/mostly/stackerr"
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
	call, err := chat.ReadToolCall[fileArgs](ctx, caller, toolCallIndex)
	if err != nil {
		msg := *chatmessages.New(llm.Message{Role: "tool", Content: fmt.Sprintf("Error parsing args: %+v", err)})
		return chatmessages.ChatMessages{msg}, nil
	}
	if call.Function.Arguments.Path == "" {
		msg := *chatmessages.New(llm.Message{Role: "tool", Content: "Error parsing args: missing required 'path', check the tool call and try again."})
		return chatmessages.ChatMessages{msg}, nil
	}
	if strings.Contains(call.Function.Arguments.Path, "..") {
		msg := *chatmessages.New(llm.Message{Role: "tool", Content: "Error Invalid file path. Please select a file within the permitted folder."})
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
	call, err := chat.ReadToolCall[fileArgs](ctx, caller, toolCallIndex)
	if err != nil {
		return NewResponse(call, fmt.Sprintf("Error parsing args: %+v", err)), nil
	}
	if call.Function.Arguments.Path == "" {
		return NewResponse(call, "Error parsing args: missing required 'path', check the tool call and try again."), nil
	}
	if filepath.IsAbs(call.Function.Arguments.Path) {
		rel, err := filepath.Rel(global.Dir(), call.Function.Arguments.Path)
		if err != nil {
			return nil, stackerr.Errorf("make rel: %w", err)
		}
		call.Function.Arguments.Path = rel
	}
	if strings.Contains(call.Function.Arguments.Path, "..") {
		return NewResponse(call, "Error Invalid file path. Please select a file within the permitted folder."), nil
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
