package implv2

import (
	"context"
	"fmt"

	"m/pkg/chat"
	"m/pkg/chatmessages"
)

func DoReview(ctx context.Context, convo chatmessages.A, e chat.Ext) (string, error) {
	_ = chat.Chat
	return "", fmt.Errorf("review not implemented")
}
