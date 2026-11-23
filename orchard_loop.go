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
//
// Returned convo does not have caller, and may be compressed
func (odefaults) Generator(ctx context.Context, cfg Config, convo mllm.ChatMessages) (mllm.ChatMessages, mllm.ChatMessage, error) {
	//	startLen := len(convo) // for system prompt
	sfn := cfg.DefaultSystemPrompt
	if cfg.SystemPromptFunc != nil {
		sfn = cfg.SystemPromptFunc
	}
	if mllm.DebugLLM {
		Slog().Info("convo", "size", len(convo), "top", convo.Top())
	}
	var sprompt string // new sticky system prompt each turn
	var err error

	// generate new sticky system prompt for this run
	sprompt, err = sfn(cfg.StateConfig)
	if cfg.PromptPrefix != "" {
		sprompt = cfg.PromptPrefix + "\n" + sprompt
	}
	if cfg.PromptSuffix != "" {
		sprompt = sprompt + "\n" + cfg.PromptSuffix
	}
	if err != nil {
		return convo, mllm.ChatMessage{}, Errorf("SystemPrompt: %w", err)
	}

	// compaction P1 (entire convo pre-limit)
	if cfg.PreTextHookEditP1 != nil {
		if convo2, err := cfg.PreTextHookEditP1(ctx, convo); err != nil {
			return convo, mllm.ChatMessage{}, Errorf("edit hook-1: %w", err)
		} else if !slices.EqualFunc(convo, convo2, func(e1, e2 mllm.ChatMessage) bool {
			return e1.MessageID.Compare(e2.MessageID) == 0 && e1.Equal(e2.Message)
		}) {
			convo = convo2
			if cfg.AfterCompress != nil {
				cfg.AfterCompress(convo) // replace convo elsewhere if P1 doesnt
			}
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
			return convo, mllm.ChatMessage{}, Errorf("edit hook-2: %w", err)
		}
	}

	// pre-text hook send signal for UI to add a new "pending" message
	if cfg.PreTextHook != nil {
		if err = cfg.PreTextHook(); err != nil {
			return convo, mllm.ChatMessage{}, Errorf("pre-text hook: %w", err)
		}
	}
	// generate a new single message. MultiChat uses cfg.E.Handlers ([Config.E]) ([mllm.E.Handlers]) to stream to UI/stdout
	var caller mllm.ChatMessage
	caller, err = cfg.E.MultiChat(ctx, convox, cfg.Think)
	if err != nil && ctx.Err() == nil {
		return convo, caller, Errorf("llm: %w", err)
	}
	if caller.Content == "" && len(caller.ToolCalls) == 0 {
		return convo, caller, Errorf("llm: no message created")
	}
	if err != nil { // add cancellation message
		caller.Content += fmt.Sprintf("\n(this was cancelled by the user: %v", context.Cause(ctx))
	}
	// aftergen add db row
	if cfg.DB != nil {
		if err = cfg.DB.Create(context.WithoutCancel(ctx), &caller); err != nil {
			return convo, caller, Errorf("db: %w", err)
		}
	}
	return convo, caller, nil
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
	/* orchard config and convo */
	ctx2 = WithConfig(ctx2, cfg)
	ctx2 = WithConvo(ctx2, slices.Clone(convo))
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
	toolout := call.ReplyTool(out).ToThreadMessage(cfg.E.From, cfg.E.ThreadID)
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

var (
	cfgctxk   ctxkey = "orchardcfg"
	convoctxk ctxkey = "orchardconvo"
)

type ctxkey string

func CtxConfig(ctx context.Context) (Config, bool) {
	cfg, ok := ctx.Value(cfgctxk).(Config)
	return cfg, ok
}

func WithConfig(ctx2 context.Context, cfg Config) context.Context {
	return context.WithValue(ctx2, cfgctxk, cfg)
}

func CtxConvo(ctx context.Context) (mllm.ChatMessages, bool) {
	convo, ok := ctx.Value(convoctxk).(mllm.ChatMessages)
	return convo, ok
}

func WithConvo(ctx2 context.Context, convo mllm.ChatMessages) context.Context {
	return context.WithValue(ctx2, convoctxk, convo)
}
