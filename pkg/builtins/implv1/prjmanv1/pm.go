package prjmanv1

import (
	"context"

	"m/pkg/chatmessages"
	"m/pkg/llm"
)

func ProjectManagerStart(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
	return nil, nil
}

func SplitStack(from string, stack chatmessages.ChatMessages) []chatmessages.ChatMessages {
	var ss []chatmessages.ChatMessages
	for len(stack) != 0 {
		for i := range stack {
			l := len(ss)
			if l != 0 && ss[l-1].TopAssistant().From != from {
				ss = append(ss, stack[:i])
			}
		}
	}
	return ss
}
