package implv2

import (
	"context"
	"encoding/json"
	"fmt"

	"m/pkg/chatmessages"
	"m/pkg/llm"
)

type haspath struct {
	Path string `json:"path"`
}

func EditTool(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
	path := getpath(caller.ToolCalls[toolCallIndex])
	// begin loop

	msg := llm.Message{
		Role:     "tool",
		Content:  "no output",
		ToolName: "edit_file", ToolCallID: caller.ToolCallID,
	}
	if path == "" {
		msg.Content = "Error: path required"
		return chatmessages.A{*chatmessages.New(msg)}, nil
	}

	// begin loop
	var out chatmessages.A

	var operators []string
	x := jsonstring(operators)
	msg.Content = fmt.Sprintf("The File edit session (%q) has completed. Transcript: %s", path, x)
	out = append(out, *chatmessages.New(msg))
	return out, nil
}

func getpath(rawMessage json.RawMessage) string {
	return ""
}

func jsonstring(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func foo(stack chatmessages.A) chatmessages.A {
	beginLength := len(stack)
	x := chatmessages.New(llm.Message{Role: "system", Content: SystemInstruction()})
	x.Ephemeral = true

	return stack[beginLength:]
}
