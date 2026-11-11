// Package orchard tiny ai agent
package orchard

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"slices"
	"time"
	"uuid"

	"github.com/aerth/mostly/mllm"
	"gorm.io/gorm"
)

type Config struct {
	mllm.E // llm client

	DB gorm.CreateInterface[mllm.ChatMessage] // Optional, get with: gorm.G[mllm.ChatMessage](db)

	Toolbox mllm.Tools // Required: Tools available (must not be empty)

	Think any // probably false or "low" or "none"

	LimitHistory int // loop option, infinite < 0
	LimitRounds  int // loop option, infinite < 0

	// ThreadReader loads the history ONCE per [Create] run. see [NewStaticThreadReader]
	ThreadReader func(ctx context.Context, threadID uuid.UUID) (mllm.ChatMessages, error)

	// SystemPrompt might even return a const string
	SystemPrompt func(*PromptInput) (string, error) // sticky system prompt

	NonTextHook func(mllm.Message) error // Hook: tool outputs after running tool
	PreTextHook func() error             // Hook: before starting generate stream
	FinishHook  func()                   // Hook: after [Create] is finished with all rounds
}

// PromptInput new subject to change
type PromptInput struct {
	ToolNames []string
	Extra     string // added to internal prompt
}

func Flags() {
	// currently no flags
}

func NewStringThreadReader(userSingleMessage string) ThreadReader {
	a := mllm.ChatMessages{{Role: "user", Content: userSingleMessage}}
	return func(ctx context.Context, _ uuid.UUID) (mllm.ChatMessages, error) {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		return a, nil
	}
}

func NewStaticThreadReader(a mllm.ChatMessages) ThreadReader {
	return func(ctx context.Context, _ uuid.UUID) (mllm.ChatMessages, error) {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		return a, nil
	}
}

// ThreadReader function is the way to load the history, at beginning of each [Create]
type ThreadReader = func(ctx context.Context, threadID uuid.UUID) (mllm.ChatMessages, error)

// DefaultSystemPrompt function for [Create] if not provided in [Config]
var DefaultSystemPrompt = sysprompt

//go:embed systemprompt.md
var systemprompttemplate string

func sysprompt(input *PromptInput) (string, error) {
	if input == nil {
		input = &PromptInput{}
	}
	wd, _ := os.Getwd()
	return executeTemplateString(systemprompttemplate, templateData{
		Tools:            input.ToolNames,
		WorkingDirectory: wd,
		CurrentTime:      time.Now().Truncate(time.Second).UTC().Format(time.DateTime),
		Extra:            input.Extra,
	})
}

type templateData struct {
	WorkingDirectory string
	CurrentTime      string
	Tools            []string
	Extra            string
}

func (t templateData) HasTool(name string) bool {
	return slices.Contains(t.Tools, name)
}

func executeTemplateString(s string, d templateData) (string, error) {
	var buf bytes.Buffer
	t := template.Must(template.New("").Parse(s))
	if err := t.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

var Slog = slog.Default

// GormThreadReader uses a sql database to read the conversation thread by threadID
//
// see [mllm.ChatMessage]
func (cfg Config) GormThreadReader(ctx context.Context, threadID uuid.UUID) (convo mllm.ChatMessages, err error) {
	if cfg.DB == nil {
		return nil, fmt.Errorf("no db")
	}
	if cfg.LimitHistory == 0 {
		cfg.LimitHistory = 100
	}
	convo, err = cfg.DB.Where("thread = ?", threadID.String()).Order("created_at desc").Limit(cfg.LimitHistory).Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("db: thread %s, %w", threadID, err)
	}
	convo = slices.DeleteFunc(convo, func(v mllm.ChatMessage) bool {
		return v.IsBlank() || v.Aux != nil // only llm here
	})
	if len(convo) == 0 {
		slog.Info("welcome", "thread", threadID.String(), "convo", len(convo)) // cant return error here
	}
	slices.Reverse(convo)
	return convo, nil
}

// Create is the main loop. Although it returns the new messages, these should be gathered using the 3 hooks in [Config]
func Create(ctx context.Context, threadID uuid.UUID, cfg Config) (mllm.ChatMessages, error) {
	if cfg.ThreadID == uuid.Nil() {
		cfg.ThreadID = threadID
	}
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
		return nil, fmt.Errorf("threadreader: no convo for thread %s", threadID)
	}
	startLen := len(convo) // for system prompt
	var caller mllm.ChatMessage
	promptinput := &PromptInput{
		ToolNames: Names(cfg.Toolbox),
	}
Loop:
	sfn := cfg.SystemPrompt
	if sfn == nil {
		sfn = DefaultSystemPrompt
	}
	sprompt, err := sfn(promptinput)
	if err != nil {
		return nil, fmt.Errorf("SystemPrompt: %w", err)
	}
	if mllm.DebugLLM {
		Slog().Info("convo", "size", len(convo), "top", convo.Top())
	}
	convox := append(mllm.ChatMessages{mllm.ChatMessage{
		Role:    "system",
		Content: sprompt,
	}}, convo...)
	convox = slices.DeleteFunc(convox, func(v mllm.ChatMessage) bool {
		return v.IsBlank() || v.Aux != nil
	})

	if cfg.PreTextHook != nil {
		if err := cfg.PreTextHook(); err != nil {
			return convo[startLen:], fmt.Errorf("nontext hook: %w", err)
		}
	}
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
		if cfg.FinishHook != nil {
			cfg.FinishHook()
		}
		return convo[startLen:], nil
	}
	calls := caller.ToolCalls
	if mllm.DebugLLM {
		Slog().Info("running tools", "calls", len(calls), "convo_size", len(convo), "top", convo.Top())
	}
	runner := cfg.Toolbox
	for index, v := range calls {
		caller.ToolCalls = []json.RawMessage{v}
		var call mllm.Call[json.RawMessage]
		err = json.Unmarshal(v, &call)
		if err != nil {
			return convo[startLen:], fmt.Errorf("json decode call: %w", err)
		}
		var out any

		/* mllm call stack */
		ctx2 := ctx
		ctx2 = mllm.WithCaller(ctx2, caller.Message)
		ctx2 = mllm.WithCall(ctx2, call)
		ctx2 = mllm.WithCallIndex(ctx2, index)
		ctx2 = mllm.WithClient(ctx2, cfg.E)
		mp := call.Function.ArgsToMap()
		if len(mp) == 0 {
			mp = nil
		}
		out, err = runner.Run(ctx2, call.Function.Name, mp)
		if err != nil {
			slog.Error("tool run", "err", err, "name", call.Function.Name)
			out = fmt.Errorf("json decode call map: %w", err)
		}
		toolout := call.ReplyTool(out).ToThreadMessage("", threadID)
		convo = append(convo, *toolout)
		toolout = &convo[len(convo)-1]
		if cfg.DB != nil {
			if err := cfg.DB.Create(ctx, toolout); err != nil {
				return convo[startLen:], fmt.Errorf("db: %w", err)
			}
		}
		if cfg.NonTextHook != nil {
			if err := cfg.NonTextHook(toolout.Message); err != nil {
				return convo[startLen:], fmt.Errorf("nontext hook: %w", err)
			}
		}
	}
	goto Loop
}

func Names(tools mllm.Tools) []string {
	names := make([]string, len(tools))
	for i := range tools {
		names[i] = tools[i].GetName()
	}
	return names
}
