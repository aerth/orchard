package gitlog

import (
	"context"

	"m/pkg/llm"
	"m/pkg/types/chatmessages"
	"m/pkg/types/otool"
)

var cmdline = "git --no-pager log -p -n 100 --reverse | tail -n 200"

var _ otool.ExecFunc = ToolExec

func ToolExec(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.A, error) {
	msg := chatmessages.NewToolReplyPlain(caller, toolCallIndex, "hi")
	return chatmessages.A{*msg}, nil
}
