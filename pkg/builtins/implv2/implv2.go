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
	fields := strings.Fields(content)
	if len(fields) == 0 {
		return nil, errors.New("empty LLM response")
	}

	// 1. Strict Leading Space Check: Content must start exactly with the first field
	op := fields[0]
	if !strings.HasPrefix(content, op) {
		return nil, errors.New("leading whitespace is strictly forbidden")
	}

	// Remove the operator prefix cleanly
	remainder := content[len(op):]

	var actions []Action

	switch op {
	case OpNextChunk, OpPrevChunk, OpCancel:
		// 2. Strict Exact-Match: Remainder must be absolutely empty
		if remainder != "" {
			return nil, errors.New("command " + op + " does not accept arguments or trailing formatting")
		}
		actions = append(actions, Action{Op: op})

	case OpGoTo:
		// 3. Strict Separator Check: Remainder must start with exactly one space, followed by a valid character
		if !strings.HasPrefix(remainder, " ") || strings.HasPrefix(remainder, "  ") || remainder == " " {
			return nil, errors.New("GOTO command must be followed by exactly one space and a target")
		}

		searchTarget := remainder[1:] // Clip the single separating space cleanly
		actions = append(actions, Action{Op: OpGoTo, Content: searchTarget})

	case OpLockIn:
		// 4. Strict Coordinate Bound Check: Remainder must start with exactly one space, followed by the coordinate field
		if !strings.HasPrefix(remainder, " ") || strings.HasPrefix(remainder, "  ") {
			return nil, errors.New("LOCK command must be followed by exactly one space")
		}

		coordinates := remainder[1:]
		// Ensure the coordinate block contains absolutely no extra parameters or punctuation
		if len(fields) != 2 || coordinates != fields[1] {
			return nil, errors.New("LOCK command requires exactly one coordinate argument with no trailing text")
		}

		// Strictly validate that coordinates only consist of digits and hyphens
		for i := 0; i < len(coordinates); i++ {
			c := coordinates[i]
			if (c < '0' || c > '9') && c != '-' {
				return nil, errors.New("LOCK arguments contain invalid characters or punctuation")
			}
		}

		actions = append(actions, Action{Op: OpLockIn, Content: coordinates})

	default:
		return nil, errors.New("invalid protocol command: " + op)
	}

	return actions, nil
}
