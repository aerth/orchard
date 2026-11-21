package compactor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/orchard/prompts"
)

var (
	CompactToolOutputLen = 1000
	CompactConvoAt       = 50
	compactionPrompt, _  = prompts.Get("compactor")
)

func V1(ctx context.Context, convo mllm.ChatMessages) (mllm.ChatMessages, error) {
	if CompactConvoAt == 0 {
		return convo, nil
	}
	if len(convo) < CompactConvoAt {
		return convo, nil
	}
	thread := convo.Top().Thread
	_ = thread // TODO: use below
	l := len(convo)
	upper := l - 5
	lower := 5
	if upper < lower {
		return convo, nil
	}
	for i := range convo[:upper] { // all but last X
		if convo[i].Role == "tool" && len(convo[i].Content) >= CompactToolOutputLen {
			convo[i].Content = fmt.Sprintf("(this tool output was auto-compacted, but can retrieved by Message ID %s)", convo[i].MessageID.String())
		}
	}
	chunksize := 10
	var newconvo mllm.ChatMessages
	var err error
	for b := 0; b < l-1; b += chunksize {
		e := min(b+chunksize, l)
		chunk := convo[b:e]
		chunk, err = compactChunk(ctx, chunk)
		if err != nil {
			return convo, err
		}
		newconvo = append(newconvo, chunk...)
	}
	return newconvo, nil
}

func compactChunk(ctx context.Context, chunk mllm.ChatMessages) (mllm.ChatMessages, error) {
	// TODO implement
	return chunk, nil
}

/* OLD:
slog.Info("begin compaction", "size", len(convo))
t1 := time.Now()
defer func() { slog.Info("end compaction", "d", time.Since(t1).String()) }()
startAt := 0
if convo[0].Role == "system" {
	// skip system message if used as P2
	startAt = 1
}
llm2 := mllm.Default()
llm2.Tools = nil // compactor needs no tools
got, err := llm2.Get(ctx, mllm.Messages{
	{Role: "system", Content: compactionPrompt},
	{Role: "user", Content: FormatConvo(convo[startAt:])},
}, false)
if err != nil {
	slog.Error("could not run compaction", "err", err)
	return convo, nil // return full convo
}
if got.Message.Content == "" {
	slog.Warn("no compression message", "got", got)
	return convo, nil
}
// keep 0,1,2 add compaction
convo = append(convo[:lower], *got.Message.ToThreadMessage("compactor", thread))
slog.Info("COMPACTED", "compacted", convo.Top().Content)
time.Sleep(time.Second * 2)
return convo, nil*/
//}

var FormatConvo = formatConvo

func formatConvo(chatMessages mllm.ChatMessages) string {
	var w bytes.Buffer
	for i, m := range chatMessages {
		fmt.Fprintf(&w, "#%d", i)
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
