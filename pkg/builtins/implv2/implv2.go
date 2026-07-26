package implv2

import "strings"

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
	var actions []Action
	fields := strings.Fields(content)
	switch fields[0] {
	case OpNextChunk:
	case OpPrevChunk:
	case OpGoTo:
	case OpLockIn:
	case OpCancel:
	}
	return actions, nil
}
