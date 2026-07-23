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

	absPath, err := filepath.Abs(call.Function.Arguments.Path)
	if err != nil {
		return NewResponse(call, fmt.Sprintf("ReadFile error (path resolution): %v", err)), nil
	}
	relPath, err := filepath.Rel(global.Dir(), absPath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return NewResponse(call, "Error: Access denied. File must be within the current working directory."), nil
	}

	data, err := os.ReadFile(absPath)
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

	absPath, err := filepath.Abs(call.Function.Arguments.Path)
	if err != nil {
		return NewResponse(call, fmt.Sprintf("ListDirectory error (path resolution): %v", err)), nil
	}
	relPath, err := filepath.Rel(global.Dir(), absPath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return NewResponse(call, "Error: Access denied. Directory must be within the current working directory."), nil
	}

	entries, err := os.ReadDir(absPath)
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

type writefileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (w writefileArgs) IsToolArgument() {}

func WriteFile(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
	// 1. Extract args from the specific tool call in the message history
	// This assumes your Message struct allows access to the raw ToolCalls string slice
	// var args writefileArgs
	call, err := chat.ReadToolCall[writefileArgs](ctx, caller, toolCallIndex)
	if err != nil {
		msg := *chatmessages.New(llm.Message{Role: "tool", Content: fmt.Sprintf("Error parsing args: %+v", err)})
		return chatmessages.ChatMessages{msg}, nil
	}
	if call.Function.Arguments.Path == "" {
		msg := *chatmessages.New(llm.Message{Role: "tool", Content: "Error parsing args: missing required 'path', check the tool call and try again."})
		return chatmessages.ChatMessages{msg}, nil
	}
	if call.Function.Arguments.Content == "" {
		msg := *chatmessages.New(llm.Message{Role: "tool", Content: "Error parsing args: missing required 'content', check the tool call and try again."})
		return chatmessages.ChatMessages{msg}, nil
	}

	absPath, err := filepath.Abs(call.Function.Arguments.Path)
	if err != nil {
		return NewResponse(call, fmt.Sprintf("WriteFile error (path resolution): %v", err)), nil
	}
	relPath, err := filepath.Rel(global.Dir(), absPath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return NewResponse(call, "Error: Access denied. File must be within the current working directory."), nil
	}

	err = os.WriteFile(absPath, []byte(call.Function.Arguments.Content), 0o600)
	if err != nil {
		return NewResponse(call, fmt.Sprintf("WriteFile error: %v", err)), nil
	}
	x := NewResponse(call, "The file was modified.")
	return x, nil
}

func NormalizePath(input string) (string, error) {
	absPath, err := filepath.Abs(input)
	if err != nil {
		return "", fmt.Errorf("path resolution error: %w", err)
	}

	relPath, err := filepath.Rel(global.Dir(), absPath)
	if err != nil || strings.HasPrefix(relPath, "..") || relPath == ".." {
		return "", fmt.Errorf("access denied: path must be within the current working directory")
	}

	return relPath, nil
}
