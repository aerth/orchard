// Package orchard tiny ai agent
package orchard

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/mostly/stackerr"
	"github.com/aerth/orchard/pkg/bashtooler"
	"github.com/aerth/orchard/pkg/builtintools"
	"github.com/aerth/orchard/pkg/orchardenv"
	"gorm.io/gorm"
)

// New creates a config. Add Tools, Hooks and call Run.
func New() Config {
	e := mllm.Default()
	x := Config{
		E:            e,
		Think:        defaults.Think,
		LimitHistory: defaults.LimitHistory,
		LimitRounds:  defaults.LimitRounds,
		Toolbox:      AllTools,
	}
	return x
}

func (cfg Config) WithTools(tools mllm.Tools) Config {
	cfg.Toolbox = tools
	return cfg
}

// Run a new multi-turn agentic run
func (cfg Config) Run(ctx context.Context, convo mllm.ChatMessages) error {
	return runConvo(ctx, cfg, convo)
}

// Config main orchard configuration. Required is: Toolbox.
type Config struct {
	E mllm.E // llm client, created by env LLM_HOST,LLM_MODEL,LLM_API_KEY

	DB gorm.CreateInterface[mllm.ChatMessage] `json:"-"` // Optional, get with: gorm.G[mllm.ChatMessage](db)

	Toolbox mllm.Tools // Required: Tools available (must not be empty. E.Tools must be nil)

	Think any // probably false or "low" or "none". If nil, is not sent, and will use model default.

	LimitHistory int // loop option, infinite less than 0 (default 20)
	LimitRounds  int // loop option, infinite less than 0 (default -1)

	PromptPrefix string // copied from [Custom.PromptPrefix]
	PromptSuffix string // copied from [Custom.PromptSuffix]

	funcConfig `json:"-"` // hooks

	StateConfig `json:"-"`

	UserConfig `json:"-"`
}
type UserConfig struct {
	UserName string // From for user messages
}

// StateConfig for sticky system prompt
type StateConfig struct {
	Role       string
	SkillNames []string
}

type funcConfig struct {
	// ThreadReader ... if using [Create]. loads the history ONCE per [Create] run. see [NewStaticThreadReader]
	// If static, this config should not be re-used and [Create] should be called ONLY once.
	//
	// Deprecated: bring your own convo somehow
	ThreadReader func(ctx context.Context, threadID uuid.UUID) (mllm.ChatMessages, error)

	// SystemPromptFunc might even return a const string and ignore input, generally should be nil to use default.
	//
	// Wrapper pseudocode: cfg.SystemPromptFunc = func(st){cfg.DefaultSystemPrompt(st)}
	SystemPromptFunc func(StateConfig) (string, error)

	PreTextHook func() error // UI hook (for example, insert new pending message to switch [mllm.StreamHandler] to)

	PreTextHookEditP1 func(ctx context.Context, convo mllm.ChatMessages) (mllm.ChatMessages, error) // Edit Convo Hook EXPERIMENTAL (for example, compaction/compression)
	PreTextHookEditP2 func(ctx context.Context, convo mllm.ChatMessages) (mllm.ChatMessages, error) // Edit Convo Hook EXPERIMENTAL (for example, compaction/compression)
	AfterCompress     func(convo mllm.ChatMessages)

	PreToolHook  func(ctx context.Context, name string, args map[string]any) error                           // Optional: can modify or deny toolcall
	PostToolHook func(ctx context.Context, name string, args map[string]any, output *mllm.ChatMessage) error // Optional: can modify or deny toolout before DB, NOT FOR UI

	NonTextHook func(mllm.Message) error          // UI Hook: for rendering tool outputs after running tool, for UI.
	FinishHook  func(fullconvo mllm.ChatMessages) // UI Hook: after [Create] is finished with all rounds
}

func (f funcConfig) SystemPrompt(st StateConfig) (string, error) {
	return f.SystemPromptFunc(st)
}

// GormThreadReader uses a sql database to read the conversation thread by threadID
//
// see [mllm.ChatMessage]
func (cfg Config) GormThreadReader(ctx context.Context, threadID uuid.UUID) (convo mllm.ChatMessages, err error) {
	if cfg.DB == nil {
		return nil, Errorf("no db")
	}
	if cfg.LimitHistory == 0 {
		cfg.LimitHistory = defaults.LimitHistory
	}
	if cfg.LimitHistory == 0 {
		cfg.LimitHistory = 100 // use limit for gorm
	}
	return unstable{}.GormThreadReader(ctx, cfg.DB, cfg.E.ThreadID, cfg.LimitHistory)
}

// Normalized config using var.go's [defaults] and [Custom]
func (cfg *Config) Normalized() error {
	if cfg.E.Tools != nil {
		return Errorf("tool spec should be nil")
	}
	if len(cfg.Toolbox) == 0 && !testing.Testing() && (cfg.E.Tools != nil) {
		return Errorf("toolbox missing")
	}
	if err := cfg.E.ValidateConfig(cfg.Think); err != nil {
		return err
	}
	_ = defaults.LimitHistory
	if cfg.LimitHistory == 0 {
		return fmt.Errorf("should set history limit in Config")
	}
	if cfg.LimitRounds == 0 {
		return fmt.Errorf("should set rounds limit in Config")
	}
	return nil
}

// Create reads ThreadReader and calls [runConvo].
//
// All output is managed by using hooks for example [Config.PreTextHook]  [Config.NonTextHook]
//
// Deprecated: use [Config.Run] and bring convo
func Create(ctx context.Context, cfg Config) error {
	if err := cfg.Normalized(); err != nil {
		return err
	}
	if cfg.ThreadReader == nil && cfg.DB != nil {
		cfg.ThreadReader = cfg.GormThreadReader
	}
	if cfg.ThreadReader == nil {
		return Errorf("unconfigured threadreader or missing db")
	}
	convo, err := cfg.ThreadReader(ctx, cfg.E.ThreadID)
	if err != nil {
		return Errorf("threadreader: %w", err)
	}
	Slog().Debug("thread_reader read convo", "len", len(convo))
	return runConvo(ctx, cfg, convo)
}

// runConvo runs the main orchard loop, using cfg [Config.Normalized].
//
// All output is managed by using Hooks, for example [Config.PreTextHook]  [Config.NonTextHook]
func runConvo(ctx context.Context, cfg Config, convo mllm.ChatMessages) error {
	if err := cfg.Normalized(); err != nil {
		return err
	}
	if len(cfg.Toolbox) != 0 {
		cfg.E.Tools = cfg.Toolbox.ToJSON()
	}
	if len(convo) == 0 {
		if cfg.FinishHook != nil {
			cfg.FinishHook(convo)
		}
		return Errorf("threadreader: no convo for thread %s", cfg.E.ThreadID)
	}
	//startLen := len(convo) // for system prompt
	var err error
	Slog().Debug("starting orchard loop", "max", cfg.LimitRounds, "max_history", cfg.LimitHistory, "think", cfg.Think, "at", len(convo))
	t1 := time.Now()
	for nround := 0; cfg.LimitRounds < 0 || nround <= cfg.LimitRounds; nround++ {
		// sleep
		if nround != 0 && sleepBetween != 0 {
			Slog().Debug("sleep", "dur", sleepBetween.String(), "round", nround, "roles", convo.Roles(), "at", len(convo))
			select {
			case <-time.After(sleepBetween):
				// ok
			case <-ctx.Done():
				if cfg.FinishHook != nil {
					cfg.FinishHook(convo)
				}
				return context.Cause(ctx)
			}
		}
		// generate
		Slog().Debug("generating", "round", nround, "roles", convo.Roles(), "at", len(convo), "since", time.Since(t1).String())
		if convo.Top().Role == "assistant" {
			return Errorf("top is assistant")
		}
		var caller mllm.ChatMessage
		t2 := time.Now()
		// generate, running all [mllm.StreamHandler] funcs
		convo, caller, err = Custom.GeneratorFunc(ctx, cfg, convo)
		if err != nil {
			if cfg.FinishHook != nil {
				cfg.FinishHook(convo) // no caller at top
			}
			return Errorf("%w", err)
		}
		convo = append(convo, caller)
		Slog().Debug("stream complete", "convo_size", len(convo), "top_id", convo.Top().MessageID, "d", time.Since(t2), "since", time.Since(t1).String())

		// NORMAL DONE EXIT
		if len(caller.ToolCalls) == 0 {
			if cfg.FinishHook != nil {
				cfg.FinishHook(convo) // with caller at top
			}
			return nil
		}
		// RUN TOOLS
		calls := caller.ToolCalls
		if mllm.DebugLLM {
			Slog().Info("running tools", "calls", len(calls), "convo_size", len(convo), "top", convo.Top())
		}
		for index := range calls { // might be multiple calls in one message
			var toolouts mllm.ChatMessages
			// add convo to toolrunner context
			ctx2 := WithConvo(ctx, slices.Clone(convo))
			// run the tool call
			toolouts, err = Custom.ToolRunnerFunc(ctx2, cfg, caller, index)
			convo = append(convo, toolouts...)
			Slog().Debug(fmt.Sprintf("tool call %d/%d complete, convo is now", index+1, len(calls)),
				"size", len(convo), "top_id", convo.Top().MessageID)
			if err != nil {
				if cfg.FinishHook != nil {
					cfg.FinishHook(convo)
				}
				return stackerr.Errorf("%w", err)
			}
			name := convo.Top().ToolName
			cmd, hasToolHook := Custom.PostToolHookCommands[name]
			if !hasToolHook && index == len(calls)-1 { // TODO: skip multi hook with multi tool call?
				cmd, hasToolHook = Custom.PostToolHookCommands["*"]
			}
			if hasToolHook { // run linter/etc
				convo, err = Custom.PostToolHookFunc(ctx, cfg, convo, cmd) // TODO: ctx2 here?
				if err != nil {
					if cfg.FinishHook != nil {
						cfg.FinishHook(convo)
					}
					return stackerr.Errorf("%w", err)
				}
			}
		}

	}

	// LIMIT WAS REACHED, YIELD
	if cfg.FinishHook != nil {
		cfg.FinishHook(convo)
	}
	return nil
}

var sleepBetween time.Duration // for testing

func (odefaults) PostToolHook(ctx context.Context, cfg Config, convo mllm.ChatMessages, cmd string) (mllm.ChatMessages, error) {
	// make a fake run_bash tool call
	call := mllm.Call[map[string]any]{
		ID:    "call_gm0bbxs7", // TODO: make new ID? seems to work..
		Index: 0,
		Type:  "function",
		Function: mllm.Function[map[string]any]{
			Name: builtintools.BashTool().GetName(),
			Arguments: map[string]any{
				"cmdline": cmd,
			},
		},
	}.ToRaw() // ToRaw for double-encode
	hooktoolenv := orchardenv.OKENVA()
	{ // get extended hook env available
		top2 := convo.TopN(2)
		if len(top2) != 2 {
			return convo, fmt.Errorf("not enough messages woops")
		}
		caller := top2[0]
		if len(caller.ToolCalls) == 0 {
			return convo, fmt.Errorf("didnt find calls woops")
		}
		toolcall := caller.ToolCalls[0] // TODO: multi, might need change sig
		var realcall mllm.Call[json.RawMessage]
		if err := json.Unmarshal(toolcall, &realcall); err != nil {
			return convo, err
		}
		toolname := realcall.Function.Name
		toolargs := string(realcall.Function.Arguments)
		hooktoolenv = append(hooktoolenv, "TOOL_NAME="+toolname, "TOOL_ARGS="+toolargs) // TODO: more env
	}
	// run the command in bash
	bashout, err := bashtooler.RunBashScript(ctx, bytes.NewReader([]byte(cmd)),
		bashtooler.RunParams{
			WorkingDir: ".",
			Args:       []string{"/dev/stdin"},
			AddEnviron: hooktoolenv,
			RawOnly:    true, // empty = skip adding to convo
		},
	)
	if err != nil { // not a non-zero exit
		return convo, fmt.Errorf("post-tool hook %q: %w", cmd, err)
	}
	if len(strings.TrimSpace(bashout)) == 0 {
		Slog().Debug("toolhook no output ✅")
		return convo, nil // minify
	}
	// add assistant-tool combo for parser
	js, _ := json.Marshal(string(call.Function.Arguments))
	call.Function.Arguments = js // double-encode args for /v1/ endpoint
	callstring := call.ToJSON()
	convo = append(convo,
		*mllm.Message{Role: "assistant", ToolCalls: []json.RawMessage{callstring}}.
			ToThreadMessage("auto", cfg.E.ThreadID),
		*mllm.ReplyTool(call, bashout).
			ToThreadMessage("auto", cfg.E.ThreadID),
	)
	if cfg.NonTextHook != nil { // TODO: use pretext hook and streamhandler? need test with TUI
		if err := cfg.NonTextHook(convo[len(convo)-2].Message); err != nil {
			return convo, err
		}
	}
	if cfg.NonTextHook != nil {
		if err := cfg.NonTextHook(convo[len(convo)-1].Message); err != nil {
			return convo, err
		}
	}
	Slog().Info("added toolhook outputs", "top", convo.TopN(2))
	return convo, nil
}
