package orchard

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"uuid"

	"github.com/aerth/mostly/mllm"
	"gorm.io/gorm"
)

type Config struct {
	mllm.E
	DB           gorm.CreateInterface[mllm.ChatMessage]
	Toolbox      mllm.Tools
	Think        any
	LimitHistory int
	LimitRounds  int
	ThreadReader ThreadReader
	SystemPrompt func() (string, error)
}

type ThreadReader = func(ctx context.Context, threadID uuid.UUID) (mllm.ChatMessages, error)

func (cfg Config) GormThreadReader(ctx context.Context, threadID uuid.UUID) (convo mllm.ChatMessages, err error) {
	if cfg.DB == nil {
		return nil, fmt.Errorf("no db")
	}
	convo, err = cfg.DB.Where("thread = ?", threadID).Order("created_at desc").Limit(cfg.LimitHistory).Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("db: %w", err)
	}
	slices.Reverse(convo)
	return convo, nil
}

func Create(ctx context.Context, threadID uuid.UUID, cfg Config) (mllm.ChatMessages, error) {
	if cfg.Tools != nil {
		return nil, fmt.Errorf("tool spec should be nil")
	}
	if len(cfg.Toolbox) == 0 {
		return nil, fmt.Errorf("toolbox missing")
	}
	if cfg.ValidateConfig(false) != nil {
		e := mllm.Default()
		cfg.E = e
	}
	cfg.Tools = cfg.Toolbox.ToJSON()
	think := any(false)
	if cfg.Think != nil {
		think = cfg.Think
	}
	if err := cfg.ValidateConfig(think); err != nil {
		return nil, err
	}
	if cfg.LimitHistory <= 0 {
		cfg.LimitHistory = 20
	}
	var convo mllm.ChatMessages
	var err error
	threadReader := cfg.ThreadReader
	if threadReader == nil && cfg.DB != nil {
		threadReader = cfg.GormThreadReader
	}
	if threadReader == nil {
		return nil, fmt.Errorf("unconfigured threadreader")
	}
	convo, err = threadReader(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("threadreader: %w", err)
	}
	if len(convo) == 0 {
		return nil, fmt.Errorf("no convo")
	}
	startLen := len(convo) + 1 // for system prompt
	var caller mllm.ChatMessage
Loop:
	sprompt, err := cfg.SystemPrompt()
	if err != nil {
		return nil, err
	}
	slog.Info("convo", "size", len(convo))
	convox := append(mllm.ChatMessages{mllm.ChatMessage{
		Role:    "system",
		Content: sprompt,
	}}, convo...)
	caller, err = cfg.MultiChat(ctx, convox, think)
	if err != nil {
		return convo[startLen:], fmt.Errorf("llm: %w", err)
	}
	if cfg.DB != nil {
		if err := cfg.DB.Create(ctx, &caller); err != nil {
			return convo[startLen:], fmt.Errorf("db: %w", err)
		}
	}
	convo = append(convo, caller)
	if len(caller.ToolCalls) == 0 {
		return convo[startLen:], nil
	}
	slog.Info("convo", "size", len(convo), "top", convo.Top())
	calls := caller.ToolCalls
	slog.Info("running tools", "calls", len(calls))

	runner := cfg.Toolbox
	for _, v := range calls {
		caller.ToolCalls = []json.RawMessage{v}
		var call mllm.Call[json.RawMessage]
		err = json.Unmarshal(v, &call)
		if err != nil {
			return convo[startLen:], fmt.Errorf("json decode call: %w", err)
		}
		var out any
		out, err = runner.Run(ctx, call.Function.Name, call.Function.ArgsToMap())
		if err != nil {
			return convo[startLen:], fmt.Errorf("json decode call: %w", err)
		}
		toolout := call.ReplyTool(out).ToThreadMessage("", threadID)
		convo = append(convo, toolout)
		if cfg.DB != nil {
			if err := cfg.DB.Create(ctx, &toolout); err != nil {
				return convo[startLen:], fmt.Errorf("db: %w", err)
			}
		}
	}
	goto Loop
}
