package toolrunner

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"m/pkg/chatmessages"
	"m/pkg/otool"

	"github.com/aerth/mostly/stackerr"
)

var ErrFatalToolResult = errors.New("unrecoverable error")

func Runtools(ctx context.Context, tools otool.Tools, caller chatmessages.ChatMessage) (chatmessages.ChatMessages, error) {
	toolouts, err := realruntools(ctx, tools, caller)
	for i := range toolouts {
		repair(caller, &toolouts[i])
	}
	return toolouts, err
}

func realruntools(ctx context.Context, tools otool.Tools, caller chatmessages.ChatMessage) (chatmessages.ChatMessages, error) {
	var toolouts chatmessages.ChatMessages
	for toolCallIndex := range caller.ToolCalls {
		var call struct {
			ID       string `json:"id"`
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		}
		if err := json.Unmarshal(caller.ToolCalls[toolCallIndex], &call); err != nil {
			return toolouts, stackerr.Errorf("%w", err)
		}
		if call.ID == "" {
			return toolouts, stackerr.Errorf("call is missing ID")
		}
		if call.Function.Name == "" {
			return toolouts, stackerr.Errorf("call is missing call.function.name")
		}
		var tool otool.Tool
		for _, t := range tools {
			if t.Name == call.Function.Name {
				tool = t
				break
			}
		}
		if tool.Name != call.Function.Name {
			slog.Warn("Not Running tools...", "i", toolCallIndex, "call", call)
			return toolouts, stackerr.Errorf("cant find tool named %s", call.Function.Name)
		}
		slog.Info("Running tools...", "caller", caller.String(), "i", toolCallIndex, "call", call)
		x, err := tool.Exec(ctx, caller.Message, toolCallIndex)
		for i := range x {
			if x[i].Role == "" {
				x[i].Role = "tool"
			}
			if x[i].Role == "tool" && x[i].ToolName == "" {
				x[i].ToolName = tool.Name
			}
			if x[i].Role == "tool" && x[i].ToolCallID == "" {
				x[i].ToolCallID = call.ID
			}
		}
		toolouts = append(toolouts, x...)
		if err != nil {
			return toolouts, err
		}

	}
	return toolouts, nil
}
