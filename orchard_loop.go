package orchard

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"

	"github.com/aerth/mostly/mllm"
)

// _ PromptFunc    = odefaults{}.PromptFunc
// var _ GeneratorFunc = odefaults{}.Generator

type odefaults struct{}

// Generator implements [GeneratorFunc] for the default in [CreateConvo].
func (odefaults) Generator(ctx context.Context, cfg Config, convo mllm.ChatMessages) (mllm.ChatMessage, error) {
	//	startLen := len(convo) // for system prompt
	sfn := cfg.SystemPrompt
	if cfg.SystemPromptFunc != nil {
		sfn = cfg.SystemPromptFunc
	}
	if mllm.DebugLLM {
		Slog().Info("convo", "size", len(convo), "top", convo.Top())
	}
	var sprompt string // new sticky system prompt each turn
	var err error
	sprompt, err = sfn()
	if cfg.PromptPrefix != "" {
		sprompt = cfg.PromptPrefix + "\n" + sprompt
	}
	if cfg.PromptSuffix != "" {
		sprompt = sprompt + "\n" + cfg.PromptSuffix
	}
	var caller mllm.ChatMessage
	if err != nil {
		return caller, Errorf("SystemPrompt: %w", err)
	}

	convox := append(mllm.ChatMessages{mllm.ChatMessage{
		Role:    "system",
		Content: sprompt,
	}}, convo...)
	convox = slices.DeleteFunc(convox, func(v mllm.ChatMessage) bool {
		return v.IsBlank() || v.Aux != nil
	})
	convox = convox.TopN(cfg.LimitHistory) // limit history

	if cfg.PreTextHook != nil {
		if err = cfg.PreTextHook(); err != nil {
			return caller, Errorf("nontext hook: %w", err)
		}
	}
	// gen
	caller, err = cfg.MultiChat(ctx, convox, cfg.Think)
	if err != nil {
		return caller, Errorf("llm: %w", err)
	}
	// aftergen
	if cfg.DB != nil { // DB.Create gives db id, do first
		if err = cfg.DB.Create(ctx, &caller); err != nil {
			return caller, Errorf("db: %w", err)
		}
	}

	return caller, nil
}

// DefaultSystemPrompt function for [Create] if not provided in [Config.SystemPrompt]
//
// A minimal default is provided that should be sufficient for basic testing.

// ToolRunner implements [ToolRunnerFunc] for the default in [CreateConvo].
//
// NOTE: runs PreToolHook and PostToolHook and finally NonTextHook
//
// TODO: simplify further
//
// TODO: hook args prevent moving the hooks outside of this func for now
func (odefaults) ToolRunner(ctx context.Context, cfg Config, caller mllm.ChatMessage, index int) (mllm.ChatMessages, error) {
	runner := cfg.Toolbox
	var convo mllm.ChatMessages
	if len(caller.ToolCalls) <= index {
		return nil, Errorf("out of range %d:%d", index, len(caller.ToolCalls))
	}
	v := caller.ToolCalls[index]
	caller.ToolCalls = []json.RawMessage{v}
	var call mllm.Call[json.RawMessage]
	err := json.Unmarshal(v, &call)
	if err != nil {
		return convo, Errorf("json decode call: %w", err)
	}

	/* mllm call stack */
	ctx2 := ctx
	ctx2 = mllm.WithCaller(ctx2, caller.Message)
	ctx2 = mllm.WithCall(ctx2, call)
	ctx2 = mllm.WithCallIndex(ctx2, index)
	ctx2 = mllm.WithClient(ctx2, cfg.E)
	mp := call.Function.ArgsToMap()

	// call it
	var out any
	if cfg.PreToolHook != nil {
		if err := cfg.PreToolHook(ctx2, call.Function.Name, mp); err != nil {
			return convo, Errorf("%w", err)
		}
	}
	out, err = runner.Run(ctx2, call.Function.Name, mp)
	if err != nil {
		slog.Error("tool run", "err", err, "name", call.Function.Name)
		out = Errorf("json decode call map: %w", err)
	}

	// prepare toolout message (call id, etc)
	toolout := call.ReplyTool(out).ToThreadMessage(cfg.From, cfg.ThreadID)
	if cfg.PostToolHook != nil {
		if err := cfg.PostToolHook(ctx, call.Function.Name, mp, toolout); err != nil {
			return convo, Errorf("%w", err)
		}
	}
	convo = append(convo, *toolout)
	toolout = &convo[len(convo)-1]
	if cfg.DB != nil { // send toolout to db
		if err := cfg.DB.Create(ctx, toolout); err != nil {
			return convo, Errorf("db: %w", err)
		}
	}
	if cfg.NonTextHook != nil { // send toolout to ui
		if err := cfg.NonTextHook(toolout.Message); err != nil {
			return convo, Errorf("nontext hook: %w", err)
		}
	}
	return convo, nil
}
