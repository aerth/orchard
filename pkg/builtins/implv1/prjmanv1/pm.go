// Package projmanv1 ...
package prjmanv1

import (
	"context"
	"strings"

	"m/pkg/chatmessages"
)

// SplitStack splits a stack of chat messages into chunks.
// The latest chunk in the returned slice is always the current conversation (the end of the original stack).
// A new chunk is started whenever an assistant message's 'From' field does not match the provided 'from' string.
func SplitStack(from string, stack chatmessages.ChatMessages) []chatmessages.ChatMessages {
	if len(stack) == 0 {
		return nil
	}

	var chunks []chatmessages.ChatMessages
	lastSplitIdx := 0

	// We iterate through the stack to find split points.
	// A split occurs if:
	// 1. The current message is an assistant message.
	// 2. Its "From" field does not match the target 'from' string.
	for i, msg := range stack {
		// Check if this message triggers a split condition
		// We look for the Assistant role where From != from
		if msg.Role == "assistant" && msg.From != from {
			// If we found a split point and it's not at the very beginning,
			// slice from the last known start to this message.
			if i > lastSplitIdx {
				chunks = append(chunks, stack[lastSplitIdx:i])
			}
			lastSplitIdx = i
		}
	}

	// Always add the remaining part of the stack as the final chunk.
	// This ensures the "latest chunk is the current conversation" requirement.
	if lastSplitIdx < len(stack) {
		chunks = append(chunks, stack[lastSplitIdx:])
	}

	return chunks
}

const PMFROM = "project_manager"

// ProjectManagerStart TODO: tests
func ProjectManagerStart(ctx context.Context, stack chatmessages.ChatMessages) (chatmessages.ChatMessages, error) {
	ss := SplitStack(PMFROM, stack)
	convo := ss[0]
	var history []chatmessages.ChatMessages
	var historyMessage []string
	{
		if l := len(ss); l != 1 {
			convo = ss[l-1]
			history = ss[:l]
			historyMessage = make([]string, len(history))
			for i := range history {
				historyMessage[i] = history[i].FormatText("", "")
			}
		}
	}
	var prefix, historymsg, suffix string
	if len(historyMessage) != 0 {
		prefix = "The following is a transcript of previous conversation."
		historymsg = strings.Join(historyMessage, "\n\n-----\n\n")
		suffix = "The following is the current conversation messages."
	}
	return append(chatmessages.ChatMessages{*chatmessages.NewSystemEphemeral(prefix + "\n\n" + historymsg + "\n\n" + suffix)}, convo...), nil
}
