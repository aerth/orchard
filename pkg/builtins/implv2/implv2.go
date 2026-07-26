package implv2

import (
	"errors"
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
	// 1. Zero tolerance for leading whitespace variations
	fields := strings.Fields(content)
	if len(fields) == 0 {
		return nil, errors.New("empty LLM response")
	}

	// 2. Strict Operator Match: Must be exactly the uppercase command keyword
	op := fields[0]

	var actions []Action

	switch op {
	case OpNextChunk, OpPrevChunk, OpCancel:
		// These commands must be exactly one word. Any extra words mean the LLM generated junk.
		if len(fields) > 1 {
			return nil, errors.New("command " + op + " does not accept arguments")
		}
		actions = append(actions, Action{Op: op})

	case OpGoTo:
		if len(fields) < 2 {
			return nil, errors.New("GOTO command requires a search target")
		}

		// Slice out the exact trailing search query.
		// Because we tolerate no leading whitespace, the operator is guaranteed to start at index 0.
		searchTarget := content[len(OpGoTo):]

		// Enforce exactly one separating space character between "GOTO" and the payload
		if !strings.HasPrefix(searchTarget, " ") {
			return nil, errors.New("GOTO command must be followed by exactly one space")
		}
		searchTarget = searchTarget[1:] // Drop the single separating space

		actions = append(actions, Action{Op: OpGoTo, Content: searchTarget})

	case OpLockIn:
		// LOCK must have exactly two fields: "LOCK" and the coordinate token (e.g., "101-103")
		if len(fields) != 2 {
			return nil, errors.New("LOCK command requires exactly one coordinate argument")
		}

		// Enforce exactly one separating space character between "LOCK" and the coordinate
		expectedPrefix := OpLockIn + " "
		if !strings.HasPrefix(content, expectedPrefix) {
			return nil, errors.New("LOCK command must be followed by exactly one space")
		}

		coordinates := fields[1]
		actions = append(actions, Action{Op: OpLockIn, Content: coordinates})

	default:
		// Any conversational text or inline code blocks without a command will fail immediately
		return nil, errors.New("invalid protocol command: " + op)
	}

	return actions, nil
}
