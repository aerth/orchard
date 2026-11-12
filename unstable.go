package orchard

import (
	"context"
	"log/slog"
	"slices"
	"uuid"

	"github.com/aerth/mostly/mllm"
	"gorm.io/gorm"
)

type unstable struct{}

/*

// ThreadReaderFunc function is the way to load the history, at beginning of each [Create]
type ThreadReaderFunc = func(ctx context.Context, threadID uuid.UUID) (mllm.ChatMessages, error)

var (
	_ ThreadReaderFunc = (Config{}.GormThreadReader)
	_ ThreadReaderFunc = (Config{}.ThreadReader)
	//_ PromptFunc   = (Config{}.SystemPromptFunc)
)
*/
// type PromptFunc = func(input *CustomPromptInput) (systemPrompt string, err error) // sticky system prompt
//func (odefaults) PromptFunc(input *CustomPromptInput) (string, error) {
//	return sysprompt(input)
//}

func (unstable) Names(tools mllm.Tools) []string {
	//_ = tools.Names // TODO: mllm.Tools.Names
	names := make([]string, len(tools))
	for i := range tools {
		names[i] = tools[i].GetName()
	}
	return names
}

func (unstable) GormThreadReader(ctx context.Context, db gorm.CreateInterface[mllm.ChatMessage], threadID uuid.UUID, limit int) (mllm.ChatMessages, error) {
	convo, err := db.Where("thread = ?", threadID.String()).Order("created_at desc").Limit(limit).Find(ctx)
	if err != nil {
		return nil, Errorf("db: thread %s, %w", threadID, err)
	}
	convo = slices.DeleteFunc(convo, func(v mllm.ChatMessage) bool {
		return v.IsBlank() || v.Aux != nil // only llm here
	})
	if len(convo) == 0 {
		slog.Info("welcome to new conversation", "thread", threadID.String(), "convo", len(convo)) // cant return error here
	}
	slices.Reverse(convo)
	return convo, nil
}

/*
// SystemPrompter maker. Might contain current time, current working directory, etc.
type SystemPrompter interface {
	SystemPrompt() (string, error)
}
type SystemPromptFunc = func() (string, error)
var _ SystemPromptFunc = funcConfig{}.SystemPromptFunc
*/
