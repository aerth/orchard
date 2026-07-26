package implv2

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Session struct {
	ActiveFile  string   // Selected filename
	Lines       []string // The cached file content split by line
	CurrentLine int      // The top line number currently visible in the chunk window
	ChunkSize   int      // How many lines to send in a chunk (e.g., 50)

	// Lock bounds (1-based line numbers)
	LockStart int
	LockEnd   int
}

// Global integer status codes for clear systemic orchestration
const (
	CodeNavigated = 200
	CodeLocked    = 201
	CodeCancelled = 202
	CodeError     = 400
)

func (s *Session) RunAction(action Action) Result {
	switch action.Op {
	case OpFilename:
		// Read file safely into memory snapshot
		data, err := os.ReadFile(action.Content)
		if err != nil {
			return Result{E: fmt.Errorf("failed to open file: %w", err), Code: CodeError}
		}

		s.ActiveFile = action.Content
		// Normalize newlines and preserve perfect line-by-line sizing
		s.Lines = strings.Split(string(data), "\n")
		s.CurrentLine = 1 // Reset viewport tracking
		s.LockStart, s.LockEnd = 0, 0
		return Result{Code: CodeNavigated}

	case OpNextChunk:
		if s.ActiveFile == "" {
			return Result{E: errors.New("no active file selected"), Code: CodeError}
		}
		if s.CurrentLine+s.ChunkSize <= len(s.Lines) {
			s.CurrentLine += s.ChunkSize
		}
		return Result{Code: CodeNavigated}

	case OpPrevChunk:
		if s.ActiveFile == "" {
			return Result{E: errors.New("no active file selected"), Code: CodeError}
		}
		s.CurrentLine -= s.ChunkSize
		if s.CurrentLine < 1 {
			s.CurrentLine = 1
		}
		return Result{Code: CodeNavigated}

	case OpGoTo:
		if s.ActiveFile == "" {
			return Result{E: errors.New("no active file selected"), Code: CodeError}
		}
		// Search the file slice for the exact matching code text
		targetLine := -1
		for idx, line := range s.Lines {
			if strings.Contains(line, action.Content) {
				targetLine = idx + 1 // Convert to 1-based indexing
				break
			}
		}
		if targetLine == -1 {
			return Result{E: fmt.Errorf("search target %q not found", action.Content), Code: CodeError}
		}

		// Shift viewport to center around the found line target
		s.CurrentLine = targetLine - (s.ChunkSize / 2)
		if s.CurrentLine < 1 {
			s.CurrentLine = 1
		}
		return Result{Code: CodeNavigated}

	case OpLockIn:
		if s.ActiveFile == "" {
			return Result{E: errors.New("no active file selected"), Code: CodeError}
		}

		// Unpack coordinate arguments safely
		parts := strings.Split(action.Content, "-")
		var start, end int
		var err error

		if len(parts) == 1 {
			start, err = strconv.Atoi(parts[0])
			end = start
		} else if len(parts) == 2 {
			start, err = strconv.Atoi(parts[0])
			end, err = strconv.Atoi(parts[1])
		} else {
			return Result{E: errors.New("invalid coordinate format"), Code: CodeError}
		}

		if err != nil || start < 1 || end < start || end > len(s.Lines) {
			return Result{E: errors.New("coordinates out of file range boundaries"), Code: CodeError}
		}

		s.LockStart = start
		s.LockEnd = end
		return Result{Code: CodeLocked}

	case OpCancel:
		s.LockStart, s.LockEnd = 0, 0
		return Result{Code: CodeCancelled}
	}

	return Result{E: errors.New("unhandled driver operator"), Code: CodeError}
}
