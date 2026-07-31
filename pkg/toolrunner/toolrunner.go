package toolrunner

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"m/pkg/chatmessages"
	"m/pkg/otool"

	"github.com/aerth/mostly/stackerr"
	"github.com/google/uuid"
)

var ErrFatalToolResult = errors.New("unrecoverable error")

const (
	CtxStack cx = "messages stack"
	CtxTools cx = "tools" // for other tools to call other tools
	CtxExt   cx = "ext"   // for other tools to call other tools
)

type cx string

func Runtools(ctx context.Context, stack chatmessages.A, tools otool.Tools, caller chatmessages.ChatMessage) (chatmessages.ChatMessages, error) {
	// should have Ext already, but stack/tools are more volatile
	if stack != nil {
		ctx = context.WithValue(ctx, CtxStack, stack)
	}
	if len(tools) != 0 {
		ctx = context.WithValue(ctx, CtxTools, tools)
	}
	toolouts, err := realruntools(ctx, tools, caller)
	return toolouts, err
}

// realruntools handles all calls
func realruntools(ctx context.Context, tools otool.Tools, caller chatmessages.ChatMessage) (chatmessages.ChatMessages, error) {
	var finishedtoolouts chatmessages.ChatMessages // outputs from all calls
	for toolCallIndex := range caller.ToolCalls {
		x, err := realruntool(ctx, tools, caller, toolCallIndex)
		slog.Info("realruntool got", "toolouts", len(x), "err", err)
		finishedtoolouts = append(finishedtoolouts, x...) // outputs from *this* message's toolcall
		if err != nil {
			return finishedtoolouts, err
		}

	}
	return finishedtoolouts, nil
}

func realruntool(ctx context.Context, tools otool.Tools, caller chatmessages.ChatMessage, toolCallIndex int) (chatmessages.ChatMessages, error) {
	var call struct {
		ID       string `json:"id"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(caller.ToolCalls[toolCallIndex], &call); err != nil {
		return nil, stackerr.Errorf("json unmarshal call: %w", err)
	}
	if call.ID == "" {
		return nil, stackerr.Errorf("call is missing ID")
	}
	if call.Function.Name == "" {
		return nil, stackerr.Errorf("call is missing call.function.name")
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
		return nil, stackerr.Errorf("cant find tool named %s in %d tools", call.Function.Name, len(tools))
	}
	slog.Info("Running tools...", "caller", caller.String(), "i", toolCallIndex, "call", call)
	x, err := tool.Exec(ctx, caller.Message, toolCallIndex)
	slog.Info("Running tools got", "toolouts", len(x), "err", err, "caller", caller.String(), "i", toolCallIndex, "call", call)
	if len(x) == 0 && err == nil {
		x = append(x, *chatmessages.NewFromRole("", "tool", "(empty)"))
	}

	// first repair toolouts
	for i := range x {
		if x[i].CreatedAt.IsZero() {
			x[i].CreatedAt = time.Now().UTC()
		}
		if x[i].ThreadID == uuid.Nil {
			x[i].ThreadID = caller.ThreadID
		}
		if x[i].MessageID == uuid.Nil {
			x[i].MessageID = uuid.New()
		}
		if x[i].Role == "" {
			x[i].Role = "tool"
		}
		if x[i].Role != "tool" {
			continue
		}
		if x[i].From == "" {
			x[i].From = caller.From
		}
		if x[i].ToolName == "" {
			x[i].ToolName = tool.Name
		}
		if x[i].ToolCallID == "" {
			x[i].ToolCallID = call.ID
		}
		if x[i].ToolName == "" || x[i].ToolCallID == "" {
			slog.Warn("bad tool repair")
		}
	}

	// then send the toolouts if we do that
	for i := range x {
		if err := AfterTool(ctx, &x[i]); err != nil {
			return x, stackerr.Errorf("%w", err)
		}
	}
	return x, nil
}

var AfterTool = func(ctx context.Context, cm *chatmessages.ChatMessage) error { return nil }
