package orchard

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"

	"github.com/aerth/mostly/mllm"
)

// _ PromptFunc    = odefaults{}.PromptFunc
// var _ GeneratorFunc = odefaults{}.Generator

type odefaults struct{}

// Generator implements [Custom.GeneratorFunc] for the default in [CreateConvo].
// Creates a single message, does not run tools. Saves to cfg.DB if exists.
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

	// generate new sticky system prompt for this run
	sprompt, err = sfn()
	if cfg.PromptPrefix != "" {
		sprompt = cfg.PromptPrefix + "\n" + sprompt
	}
	if cfg.PromptSuffix != "" {
		sprompt = sprompt + "\n" + cfg.PromptSuffix
	}
	if err != nil {
		return mllm.ChatMessage{}, Errorf("SystemPrompt: %w", err)
	}

	// compaction P1 (entire convo pre-limit)
	if cfg.PreTextHookEditP1 != nil {
		if convo, err = cfg.PreTextHookEditP1(ctx, convo); err != nil {
			return mllm.ChatMessage{}, Errorf("edit hook-1: %w", err)
		}
	}

	// get only the non-aux messages
	convox := slices.DeleteFunc(slices.Clone(convo), func(v mllm.ChatMessage) bool {
		return v.IsBlank() || v.Aux != nil
	})

	// limit history (tail)
	if cfg.LimitHistory != 0 {
		convox = convox.TopN(cfg.LimitHistory)
	}

	// new slice with sticky
	convox = append(mllm.ChatMessages{{Role: "system", Content: sprompt}}, convox...)
	// compaction P2 (preferred, only the limited convo)
	if cfg.PreTextHookEditP2 != nil {
		if convox, err = cfg.PreTextHookEditP2(ctx, convox); err != nil {
			return mllm.ChatMessage{}, Errorf("edit hook-2: %w", err)
		}
	}

	// pre-text hook send signal for UI to add a new "pending" message
	if cfg.PreTextHook != nil {
		if err = cfg.PreTextHook(); err != nil {
			return mllm.ChatMessage{}, Errorf("pre-text hook: %w", err)
		}
	}
	// generate a new single message. MultiChat uses cfg.E.Handlers ([Config.E]) ([mllm.E.Handlers]) to stream to UI/stdout
	var caller mllm.ChatMessage
	caller, err = cfg.MultiChat(ctx, convox, cfg.Think)
	if err != nil && ctx.Err() == nil {
		return caller, Errorf("llm: %w", err)
	}
	if caller.Content == "" && len(caller.ToolCalls) == 0 {
		return caller, Errorf("llm: no message created")
	}
	if err != nil { // add cancellation message
		caller.Content += fmt.Sprintf("\n(this was cancelled by the user: %v", context.Cause(ctx))
	}
	// aftergen add db row
	if cfg.DB != nil {
		if err = cfg.DB.Create(context.WithoutCancel(ctx), &caller); err != nil {
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
// TODO: simplify further: move hooks out of this replacable function. hook args prevent moving the hooks outside of this func for now [Create]
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
