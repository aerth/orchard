package mcpadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime/debug"

	"m/orchard/pkg/types/chatmessages"

	"github.com/aerth/mostly/mllm"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ExecFunc = func(ctx context.Context, caller mllm.Message, toolCallIndex int) (chatmessages.ChatMessages, error)

type NativeTool struct {
	Name        string
	Description string
	InputSchema any
	Execute     ExecFunc
}

func getRootModulePath() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Path != "" {
		return filepath.Base(info.Main.Path)
	}
	return "unknown"
}

func New() *mcp.Client {
	// 1. Create the base client instance
	client := mcp.NewClient(&mcp.Implementation{
		Name:    getRootModulePath(),
		Version: "1.0.0",
	}, nil)
	return client
}

func LoadMCPToolsAsNative(ctx context.Context, client *mcp.Client, transport mcp.Transport) ([]NativeTool, *mcp.ClientSession, error) {
	// 1. Connect and start the session
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect mcp client: %w", err)
	}

	// 2. List tools using the session
	toolsResp, err := session.ListTools(ctx, nil)
	if err != nil {
		session.Close()
		return nil, nil, fmt.Errorf("failed to list mcp tools: %w", err)
	}

	var nativeTools []NativeTool

	for _, mcpTool := range toolsResp.Tools {
		toolName := mcpTool.Name

		nativeTool := NativeTool{
			Name:        toolName,
			Description: mcpTool.Description,
			InputSchema: mcpTool.InputSchema,
			Execute: func(ctx context.Context, caller mllm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
				args, err := extractArgsFromMessage(caller, toolCallIndex)
				if err != nil {
					return nil, err
				}

				// 3. Call the tool using the session
				res, err := session.CallTool(ctx, &mcp.CallToolParams{
					Name:      toolName,
					Arguments: args,
				})
				if err != nil {
					return nil, err
				}
				if res.IsError {
					return nil, res.GetError()
				}

				return convertToChatMessages(res)
			},
		}
		nativeTools = append(nativeTools, nativeTool)
	}

	return nativeTools, session, nil
}

func extractArgsFromMessage(caller mllm.Message, index int) (map[string]any, error) {
	m := map[string]any{}
	err := json.Unmarshal(caller.ToolCalls[index], &m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func convertToChatMessages(res *mcp.CallToolResult) (chatmessages.ChatMessages, error) {
	if res.IsError {
		return nil, res.GetError()
	}

	var outputText string

	for _, contentItem := range res.Content {
		if textContent, ok := contentItem.(*mcp.TextContent); ok {
			outputText = outputText + textContent.Text
		}
	}
	return chatmessages.ChatMessages{*chatmessages.New(mllm.Message{Content: outputText, Role: "tool"})}, nil
}
