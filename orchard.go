// Package orchard tiny ai agent
package orchard

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"uuid"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/mostly/stackerr"
	"github.com/aerth/orchard/pkg/bashtooler"
	"github.com/aerth/orchard/pkg/builtintools"
	"github.com/aerth/orchard/pkg/orchardenv"
	"gorm.io/gorm"
)

// Config main orchard configuration. Required is: Toolbox.
type Config struct {
	mllm.E // llm client, created by env LLM_HOST,LLM_MODEL,LLM_API_KEY

	DB gorm.CreateInterface[mllm.ChatMessage] `json:"-"` // Optional, get with: gorm.G[mllm.ChatMessage](db)

	Toolbox mllm.Tools // Required: Tools available (must not be empty. E.Tools must be nil)

	Think any // probably false or "low" or "none". If nil, is not sent, and will use model default.

	LimitHistory int // loop option, infinite < 0
	LimitRounds  int // loop option, infinite < 0

	PromptPrefix string // copied from [Custom.PromptPrefix]
	PromptSuffix string // copied from [Custom.PromptSuffix]

	funcConfig `json:"-"` // hooks
}

type funcConfig struct {
	// ThreadReader only REQUIRED if using [Create]. loads the history ONCE per [Create] run. see [NewStaticThreadReader]
	// If static, this config should not be re-used and [Create] should be called ONLY once.
	ThreadReader func(ctx context.Context, threadID uuid.UUID) (mllm.ChatMessages, error)

	// SystemPromptFunc might even return a const string and ignore input, generally should be nil to use default.
	SystemPromptFunc func() (string, error)

	PreTextHook func() error // UI hook (for example, insert new pending message to switch [mllm.StreamHandler] to)

	PreTextHookEditP1 func(ctx context.Context, convo mllm.ChatMessages) (mllm.ChatMessages, error) // Edit Convo Hook EXPERIMENTAL (for example, compaction/compression)
	PreTextHookEditP2 func(ctx context.Context, convo mllm.ChatMessages) (mllm.ChatMessages, error) // Edit Convo Hook EXPERIMENTAL (for example, compaction/compression)

	PreToolHook  func(ctx context.Context, name string, args map[string]any) error                           // Optional: can modify or deny toolcall
	PostToolHook func(ctx context.Context, name string, args map[string]any, output *mllm.ChatMessage) error // Optional: can modify or deny toolout before DB, NOT FOR UI

	NonTextHook func(mllm.Message) error // UI Hook: for rendering tool outputs after running tool, for UI.
	FinishHook  func()                   // UI Hook: after [Create] is finished with all rounds
}

func (f funcConfig) SystemPrompt() (string, error) {
	return f.SystemPromptFunc()
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
		cfg.LimitHistory = 100
	}
	return unstable{}.GormThreadReader(ctx, cfg.DB, cfg.ThreadID, cfg.LimitHistory)
}

// Normalized config using defaults and [Custom]
func (cfg *Config) Normalized() error {
	if cfg.Tools != nil {
		return Errorf("tool spec should be nil")
	}
	if len(cfg.Toolbox) == 0 {
		return Errorf("toolbox missing")
	}
	if cfg.ValidateConfig(false) != nil {
		e := mllm.Default()
		cfg.E = e
	}
	if err := cfg.ValidateConfig(cfg.Think); err != nil {
		return err
	}
	if cfg.LimitHistory <= 0 {
		cfg.LimitHistory = defaults.LimitHistory
	}
	if cfg.LimitRounds <= 0 {
		cfg.LimitRounds = defaults.LimitRounds
	}
	return nil
}

// Create reads ThreadReader and calls [CreateConvo].
//
// Although it returns the new messages, they should be gathered using the hooks in [Config.PreTextHook]  [Config.NonTextHook]
func Create(ctx context.Context, cfg Config) (mllm.ChatMessages, error) {
	if err := cfg.Normalized(); err != nil {
		return nil, err
	}
	if cfg.ThreadReader == nil && cfg.DB != nil {
		cfg.ThreadReader = cfg.GormThreadReader
	}
	if cfg.ThreadReader == nil {
		return nil, Errorf("unconfigured threadreader or missing db")
	}
	convo, err := cfg.ThreadReader(ctx, cfg.ThreadID)
	if err != nil {
		return nil, Errorf("threadreader: %w", err)
	}
	return CreateConvo(ctx, cfg, convo)
}

// CreateConvo runs the main orchard loop, using cfg [Config.Normalized].
//
// Although it returns the new messages when finished, they should be instead gathered in real-time using the hooks in [Config.PreTextHook]  [Config.NonTextHook]
func CreateConvo(ctx context.Context, cfg Config, convo mllm.ChatMessages) (mllm.ChatMessages, error) {
	if err := cfg.Normalized(); err != nil {
		return nil, err
	}
	if len(cfg.Toolbox) != 0 {
		cfg.Tools = cfg.Toolbox.ToJSON()
	}
	defer func() {
		if cfg.FinishHook != nil {
			cfg.FinishHook()
		}
	}()
	if len(convo) == 0 {
		return nil, Errorf("threadreader: no convo for thread %s", cfg.ThreadID)
	}
	startLen := len(convo) // for system prompt
	var err error
	slog.Info("starting orchard loop", "max", cfg.LimitRounds, "max_history", cfg.LimitHistory, "think", cfg.Think, "at", startLen)
	for nround := 0; cfg.LimitRounds < 0 || nround <= cfg.LimitRounds; nround++ {
		// generate
		var caller mllm.ChatMessage
		if sleepBetween != 0 {
			slog.Debug("sleep", "dur", sleepBetween.String(), "round", nround, "roles", convo.Roles(), "at", len(convo))
			select {
			case <-time.After(sleepBetween):
				// ok
			case <-ctx.Done():
				return convo[startLen:], context.Cause(ctx)
			}
		}
		slog.Info("generating", "round", nround, "roles", convo.Roles(), "at", len(convo))
		caller, err = Custom.GeneratorFunc(ctx, cfg, convo)
		if err != nil {
			return convo[startLen:], Errorf("%w", err)
		}
		convo = append(convo, caller)
		if len(caller.ToolCalls) == 0 {
			return convo[startLen:], nil
		}
		// run tools
		calls := caller.ToolCalls
		if mllm.DebugLLM {
			Slog().Info("running tools", "calls", len(calls), "convo_size", len(convo), "top", convo.Top())
		}
		for index := range calls { // might be multiple calls in one message
			var toolouts mllm.ChatMessages
			// run the tool call
			toolouts, err = Custom.ToolRunnerFunc(ctx, cfg, caller, index)
			convo = append(convo, toolouts...)
			if err != nil {
				return convo[startLen:], stackerr.Errorf("%w", err)
			}
			name := convo.Top().ToolName
			cmd, hasToolHook := Custom.PostToolHookCommands[name]
			if !hasToolHook && index == len(calls)-1 { // TODO: multi tool call
				cmd, hasToolHook = Custom.PostToolHookCommands["*"]
			}
			if hasToolHook {
				convo, err = Custom.PostToolHookFunc(ctx, cfg, convo, cmd)
				if err != nil {
					return convo[startLen:], stackerr.Errorf("%w", err)
				}
			}
		}

	}

	return convo[startLen:], nil
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

	// run the command in bash
	bashout, err := bashtooler.RunBashScript(ctx, bytes.NewReader([]byte(cmd)),
		bashtooler.RunParams{
			WorkingDir: ".",
			Args:       []string{"/dev/stdin"},
			AddEnviron: orchardenv.OKENVA(),
			RawOnly:    true, // empty = skip adding to convo
		},
	)
	if err != nil { // not a non-zero exit
		return convo, fmt.Errorf("post-tool hook %q: %w", cmd, err)
	}
	if len(strings.TrimSpace(bashout)) == 0 {
		slog.Debug("toolhook no output ✅")
		return convo, nil // minify
	}
	// add assistant-tool combo for parser
	js, _ := json.Marshal(string(call.Function.Arguments))
	call.Function.Arguments = js // double-encode args for /v1/ endpoint
	callstring := call.ToJSON()
	convo = append(convo,
		*mllm.Message{Role: "assistant", ToolCalls: []json.RawMessage{callstring}}.
			ToThreadMessage("auto", cfg.ThreadID),
		*mllm.ReplyTool(call, bashout).
			ToThreadMessage("auto", cfg.ThreadID),
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
	slog.Info("added toolhook outputs", "top", convo.TopN(2))
	return convo, nil
}
