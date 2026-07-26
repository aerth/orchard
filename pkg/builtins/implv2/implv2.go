package implv2

type Action struct {
	Op      string
	Content string
}

const (
	OpNextChunk = "NEXT_CHUNK"
	OpPrevChunk = "PREV_CHUNK"
	OpGoTo      = "GOTO"
	OpLockIn    = "LOCK"
)

func OutputParser(content string) ([]Action, error) {
	return nil, nil
}
