package implv2

import (
	"errors"
	"fmt"
	"strings"
)

type Action struct {
	Op      string
	Content string
}

const (
	OpNextChunk = "NEXT_CHUNK"
	OpPrevChunk = "PREV_CHUNK"
	OpGoTo      = "GOTO"
	OpLockIn    = "LOCK"
	OpCancel    = "CANCEL"
)

func OutputParser(content string) ([]Action, error) {
	fields := strings.Fields(content)
	if len(fields) == 0 {
		return nil, errors.New("empty LLM response")
	}
	op := fields[0]
	var actions []Action

	switch op {
	case OpNextChunk, OpPrevChunk, OpCancel:
		actions = append(actions, Action{Op: op})

	case OpGoTo:
		if len(fields) < 2 {
			return nil, errors.New("GOTO command requires a search target")
		}
		searchTarget := strings.TrimPrefix(content, fields[0])
		if searchTarget == content {
			return nil, fmt.Errorf("invalid")
		}
		searchTarget = strings.TrimSpace(searchTarget)
		actions = append(actions, Action{Op: OpGoTo, Content: searchTarget})

	case OpLockIn:
		// LOCK requires coordinate arguments (e.g., "101-103" or "101")
		if len(fields) < 2 {
			return nil, errors.New("LOCK command requires line coordinates")
		}
		// Extract the coordinates token and drop any accidental trailing punctuation
		coordinates := strings.Trim(fields[1], "`*.,")
		actions = append(actions, Action{Op: OpLockIn, Content: coordinates})

	default:
		// If it doesn't match a command, it means the LLM is likely chatting or
		// has jumped straight into sending code blocks (if the state wasn't guarded)
		return nil, errors.New("unrecognized command sequence: " + fields[0])
	}

	return actions, nil
}
