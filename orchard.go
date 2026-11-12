// Package orchard tiny ai agent
package orchard

import (
	"context"
	_ "embed"
	"log/slog"
	"time"
	"uuid"

	"github.com/aerth/mostly/mllm"
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

	PreTextHook func() error // Optional UI Hook: before starting generate stream

	PreToolHook func(ctx context.Context, name string, args map[string]any) error // Optional: can modify or deny

	PostToolHook func(ctx context.Context, name string, args map[string]any, output *mllm.ChatMessage) error // Optional: can modify or deny before DB, NOT FOR UI

	NonTextHook func(mllm.Message) error // UI Hook: tool outputs after running tool, for UI.
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
		slog.Info("generating", "round", nround, "roles", convo.Roles(), "at", len(convo))
		if sleepBetween != 0 {
			time.Sleep(sleepBetween)
		}
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
				return convo[startLen:], err
			}

		}
	}

	return convo[startLen:], nil
}

var sleepBetween time.Duration // for testing
