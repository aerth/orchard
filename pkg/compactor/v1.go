package compactor

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"slices"
	"sync"
	"testing"
	"text/template"
	"uuid"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/mostly/ncode"
	"github.com/aerth/mostly/stackerr"
	"github.com/aerth/orchard"
	"github.com/aerth/orchard/prompts"
)

var (
	CompactToolOutputLen = 1000
	CompactConvoAtLength = 0 // disabled by default
	//	CompactConvoAtTokens = 0 // not implemented yet
	compactionPrompt = prompts.GetTemplate("compactor")
	CompressorName   = "compressor"
)

func V1(ctx context.Context, entireConvo mllm.ChatMessages) (mllm.ChatMessages, error) {
	if CompactConvoAtLength == 0 {
		return entireConvo, nil
	}
	if len(entireConvo) < CompactConvoAtLength {
		return entireConvo, nil
	}
	for _, v := range entireConvo.TopN(3) {
		if v.From == CompressorName { // TODO: prevent infinite some other way once we count token
			return entireConvo, nil
		}
	}
	l := len(entireConvo)
	upper := l - 5
	lower := 5
	if upper < lower {
		return entireConvo, nil
	}
	chunksize := 5
	if l <= chunksize {
		return entireConvo, nil
	}
	var newconvo mllm.ChatMessages
	var err error
	for b := 0; b < l; b += chunksize {
		e := min(b+chunksize, l) // -1 to keep tip
		chunk := entireConvo[b:e]

		intro := fmt.Sprintf("Conversation slice [%d:%d] of %d total", b, e, l)
		if e == l {
			intro += "\n\nThis is the final compression chunk slice. Be sure to clarify the next steps for the agent to complete the task."
		}
		chunk, err = compactChunk(ctx, intro, chunk)
		if err != nil {
			return entireConvo, err
		}
		slog.Debug("done compressing", "from", l, "to", len(chunk))
		if Debug || testing.Testing() {
			println(
				"\n\n-----\n\n",
				ncode.JSONString(l),
				ncode.JSONString(len(chunk)),
				ncode.JSONString(chunk),
				"\n\n-----\n\n",
			)
		}
		newconvo = append(newconvo, chunk...)
	}
	if len(newconvo) == 0 {
		return entireConvo, fmt.Errorf("could not compress")
	}
	if entireConvo.Top().MessageID != newconvo.Top().MessageID {
		newconvo = append(newconvo, entireConvo.Top()) // re-add most recent to keep pattern
	}
	return newconvo, nil
}

var Debug = testing.Testing() || os.Getenv("DEBUG_COMPRESSOR") == "1"

func KeepMessageTool(k *K) mllm.Tool {
	return mllm.Tool{
		Spec: mllm.Spec(`{
  "name": "keep_message",
  "description": "prevents a message from being deleted and ensures it is retained in the history",
  "parameters": {
    "type": "object",
    "properties": {
      "id": {
        "type": "string",
        "description": "the unique identifier of the message to be kept, may be comma separated list, or 'ALL'"
      }
    },
    "required": [
      "id"
    ]
  }
}`),
		Exec: k.keepExec,
	}
}

type K struct {
	keepers   map[string]bool
	convo     mllm.ChatMessages
	keepersmu sync.Mutex
}

func (k *K) keep(d string) {
	k.keepers[d] = true
}

func (k *K) Keepers() mllm.ChatMessages {
	slice := k.Slice()
	if len(slice) == 0 {
		return nil
	}
	k.keepersmu.Lock()
	defer k.keepersmu.Unlock()
	if slices.Contains(slice, "all") {
		return k.convo
	}
	convo := slices.DeleteFunc(slices.Clone(k.convo), func(v mllm.ChatMessage) bool {
		return !slices.ContainsFunc(slice, func(id string) bool {
			return v.MessageID.String() == id || v.Role == "user" || v.Role == "system"
		})
	})
	return convo
}

func (k *K) Slice() []string {
	k.keepersmu.Lock()
	defer k.keepersmu.Unlock()
	return slices.Collect(maps.Keys(k.keepers))
}

func (k *K) keepExec(ctx context.Context, args map[string]any) (any, error) {
	arg, ok := args["name"].(string)
	if !ok || arg == "" {
		return "Need 'name' parameter for keep message", nil
	}
	k.keepersmu.Lock()
	defer k.keepersmu.Unlock()
	slog.Info("compressor wants to keep", "arg", arg)
	k.keep(arg)
	msg := k.Slice()
	return fmt.Sprintf("Currently keeping: %q", msg), nil
}

type templateData struct {
	e       mllm.E
	Toolbox mllm.Tools
}

func (t templateData) HasTool(name string) bool {
	for _, v := range t.Toolbox {
		if v.GetName() == name {
			return true
		}
	}
	return false
}

func E(tpl *template.Template, data templateData) string {
	var buf bytes.Buffer
	err := tpl.Execute(&buf, data)
	if err != nil {
		panic(err.Error())
	}
	return buf.String()
}

func Flags() {
	flag.IntVar(&CompactToolOutputLen, "compact-toolout-size", CompactToolOutputLen, "compact all tool outputs that are larger than n characters (0 disable)")
	flag.IntVar(&CompactConvoAtLength, "compact-at-length", CompactConvoAtLength, "compress conversation after it grows to n messages (0 disable)")
}

func compactChunk(ctx context.Context, intro string, chunk mllm.ChatMessages, handlers ...mllm.StreamHandler) (mllm.ChatMessages, error) {
	if len(chunk) < 3 {
		return chunk, nil
	}
	var llm2 mllm.E
	if lm, ok := mllm.CtxClient(ctx); ok {
		llm2 = lm
	} else {
		llm2 = mllm.Default()
	}
	cfg, _ := orchard.CtxConfig(ctx)
	cfg.E = llm2
	for i := range chunk { // should work anyways
		if chunk[i].MessageID == uuid.Nil() {
			chunk[i].MessageID = uuid.New()
			slog.Warn("had to create a MessageID for", "role", chunk[i].Role, "at", i)
		}
	}
	threadID := chunk.Top().Thread
	slog.Info("compacting chunk", "size", len(chunk), "intro", intro)
	keeper := &K{keepers: map[string]bool{}, convo: slices.Clone(chunk)}
	for i := range chunk {
		if chunk[i].Role == "system" || chunk[i].Role == "user" {
			keeper.keep(chunk[i].MessageID.String())
		}
	}
	tools := mllm.Tools{KeepMessageTool(keeper)}
	llm2.Tools = tools.ToMarshalers()
	llm2.Handlers = append(llm2.Handlers, handlers...)
	compressionConvo := mllm.Messages{
		{Role: "system", Content: E(compactionPrompt, templateData{e: llm2, Toolbox: tools})},
		{Role: "user", Content: intro + "\n" + FormatConvo(chunk, keeper.Slice())},
	}
	if testing.Testing() {
		println(compressionConvo.Top().Content)
	}
	hasTools := true
	for i := 0; i < 10 && hasTools; i++ {
		if cfg.PreTextHook != nil {
			if err := cfg.PreTextHook(); err != nil {
				return chunk, err
			}
		}
		got, err := llm2.Get(ctx, compressionConvo, false)
		if err != nil {
			slog.Error("could not run compaction", "err", err)
			return chunk, err // return full convo
		}
		if got.Message.Content == "" {
			slog.Warn("no compression message", "got", got)
			return chunk, nil
		}
		compressionConvo = append(compressionConvo, got.Message)
		hasTools = len(got.Message.ToolCalls) != 0
		for i := range got.Message.ToolCalls {
			var call mllm.Call[map[string]any]
			err = json.Unmarshal(got.Message.ToolCalls[i], &call)
			if err != nil {
				return chunk, Errorf("unmarshal tool call %d: %w", i, err)
			}
			out, err := keeper.keepExec(ctx, call.Function.Arguments)
			if err != nil {
				return chunk, Errorf("keepExec: %w", err)
			}
			toolout := mllm.ReplyTool(call, out)
			compressionConvo = append(compressionConvo, toolout)
			if fn := cfg.NonTextHook; fn != nil {
				err = fn(toolout)
				if err != nil {
					return chunk, Errorf("NonTextHook: %w", err)
				}
			}
		}
	}
	got := compressionConvo.Top()
	got.Role = "user" // TODO: ??
	got.Content = "[compressor]\n" + got.Content
	chunk = append(keeper.Keepers(), mllm.ChatMessages{*got.ToThreadMessage(CompressorName, threadID)}...)
	return chunk, nil
}

var Errorf = stackerr.Errorf

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

func formatConvo(chatMessages mllm.ChatMessages, keepers []string) string {
	buf := new(bytes.Buffer)
	w := io.Writer(buf)
	if testing.Testing() {
		w = io.MultiWriter(buf, os.Stderr)
	}

	for _, m := range chatMessages {
		if m.MessageID == uuid.Nil() {
			panic("convo must have messageId")
		}
		x := "Marked for deletion"
		keeping := slices.Contains(keepers, m.MessageID.String())
		if keeping && m.Role == "system" { // should be
			continue
		}
		if keeping {
			x = "Keeping this message, do not include in summary"
		}
		_, _ = fmt.Fprintf(w, "\n---\nID=%s (%s)\n", m.MessageID, x)
		if m.Role == "tool" {
			if len(m.Content) > CompactToolOutputLen && !keeping {
				m.Content = "(tool output omitted for compaction process, do not keep)"
			}
			_, _ = fmt.Fprintf(w, "%s (%s): %s\n", m.Role, m.ToolName, m.Content)
			continue
		}
		if len(m.ToolCalls) != 0 {
			toolCalls, _ := json.Marshal(m.ToolCalls)
			_, _ = fmt.Fprintf(w, "%s tool_call: %s %s\n", m.Role, m.Content, string(toolCalls))
			continue
		}
		_, _ = fmt.Fprintf(w, "%s: %s\n", m.Role, m.Content)
	}
	return buf.String()
}
