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
	OpFilename  = "SELECT_FILE"
)

type Result struct {
	E    error
	Code int
}

type ActionRunner interface {
	RunAction(Action) Result
}

func RunAction(runner ActionRunner, action Action) (Result, error) {
	got := runner.RunAction(action)
	return got, got.E
}

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

	case OpGoTo, OpLockIn, OpFilename:
		if remainder == "" {
			return nil, errors.New("GOTO command must be followed by exactly one space and a target")
		}

		if op == OpLockIn {
			for i := 0; i < len(remainder); i++ {
				c := remainder[i]
				if (c < '0' || c > '9') && c != '-' {
					return nil, errors.New("LOCK arguments contain invalid characters, trailing text, or punctuation")
				}
			}
		}
		actions = append(actions, Action{Op: op, Content: remainder})
	default:
		return nil, errors.New("invalid protocol command: " + op)
	}

	return actions, nil
}
