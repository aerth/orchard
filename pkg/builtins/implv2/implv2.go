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
	var op string
	var remainder string
	var actions []Action
	{
		fields := strings.Fields(content)
		if len(fields) == 0 {
			return nil, errors.New("empty LLM response")
		}

		// 1. Strict Leading Space Check: Content must start exactly with the first field
		op = fields[0]
		if !strings.HasPrefix(content, op) {
			return nil, errors.New("leading whitespace is strictly forbidden")
		}
		// Remove the operator prefix cleanly
		if len(content) != len(op) {
			remainder = content[len(op)+1:]
		}
	}

	switch op {
	case OpNextChunk, OpPrevChunk, OpCancel: // no args
		if remainder != "" {
			return nil, errors.New("command " + op + " does not accept arguments or trailing formatting")
		}
		actions = append(actions, Action{Op: op})

	case OpGoTo:
		if remainder == "" {
			return nil, errors.New("GOTO command must be followed by exactly one space and a target")
		}
		searchTarget := remainder // Clip the single separating space cleanly
		actions = append(actions, Action{Op: OpGoTo, Content: searchTarget})

	case OpLockIn:
		// Ensure remainder starts with exactly one space and has a body
		if !strings.HasPrefix(remainder, " ") || strings.HasPrefix(remainder, "  ") || remainder == " " {
			return nil, errors.New("LOCK command must be followed by exactly one space")
		}

		coordinates := remainder

		// Strictly validate that coordinates only consist of digits and hyphens.
		// This single check automatically rejects trailing text, spaces, or punctuation!
		for i := 0; i < len(coordinates); i++ {
			c := coordinates[i]
			if (c < '0' || c > '9') && c != '-' {
				return nil, errors.New("LOCK arguments contain invalid characters, trailing text, or punctuation")
			}
		}

		actions = append(actions, Action{Op: OpLockIn, Content: coordinates})

	default:
		return nil, errors.New("invalid protocol command: " + op)
	}

	return actions, nil
}
