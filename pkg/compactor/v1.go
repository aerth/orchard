package compactor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/orchard/prompts"
)

var (
	CompactToolOutputLen = 1000
	CompactConvoAt       = 20
	compactionPrompt, _  = prompts.Get("compactor")
)

func V1(ctx context.Context, convo mllm.ChatMessages) (mllm.ChatMessages, error) {
	if len(convo) < CompactConvoAt {
		return convo, nil
	}
	thread := convo.Top().Thread
	l := len(convo)
	for i := range convo[:l-3] { // all but last 3
		if convo[i].Role == "tool" && len(convo[i].Content) >= CompactToolOutputLen {
			convo[i].Content = "(this tool output was auto-compacted)"
		}
	}
	llm2 := mllm.Default()
	slog.Info("begin compaction", "size", len(convo))
	t1 := time.Now()
	defer func() { slog.Info("end compaction", "d", time.Since(t1).String()) }()
	llm2.Tools = nil
	got, err := llm2.Get(ctx, mllm.Messages{
		{Role: "system", Content: compactionPrompt},
		{Role: "user", Content: FormatConvo(convo[1:])},
	}, false)
	if err != nil {
		slog.Error("could not run compaction", "err", err)
		return convo, nil // return full convo
	}
	// keep 0,1,2 add compaction
	convo = append(convo[:3], *got.Message.ToThreadMessage("compactor", thread))
	slog.Info("COMPACTED", "compacted", convo.Top().Content)
	time.Sleep(time.Second * 2)
	return convo, nil
}

var FormatConvo = formatConvo

func formatConvo(chatMessages mllm.ChatMessages) string {
	var w bytes.Buffer
	for _, m := range chatMessages {
		if m.Role == "tool" {
			fmt.Fprintf(&w, "%s (%s): %s\n", m.Role, m.ToolName, m.Content)
			continue
		}
		if len(m.ToolCalls) != 0 {
			toolCalls, _ := json.Marshal(m.ToolCalls)
			fmt.Fprintf(&w, "%s tool_call: %s %s\n", m.Role, m.Content, string(toolCalls))
			continue
		}
		fmt.Fprintf(&w, "%s: %s\n", m.Role, m.Content)
	}
	return w.String()
}
