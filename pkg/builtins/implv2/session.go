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

const DefaultChunkSize = 50

func (s Session) Current() []byte {
	return []byte(strings.Join(s.Lines, "\n"))
}

var ErrNot = errors.New("not an operator keyword and no LOCK range selected")

func (s Session) SystemMessage() string {
	// 1. Calculate boundaries for the active viewing chunk window
	startIdx := max(s.CurrentLine-1, 0)
	endIdx := min(startIdx+s.ChunkSize, len(s.Lines))
	// 2. Build the visual representation of the active file chunk
	var chunkBuilder strings.Builder
	if s.ActiveFile == "" {
		chunkBuilder.WriteString("[No file active. Execute SELECT_FILE <filename> to load a workspace]")
	} else {
		// Append lines with explicit absolute 1-based indices to anchor LLM awareness
		for i := startIdx; i < endIdx; i++ {
			marked := " "
			if i+1 >= s.LockStart && i+1 <= s.LockEnd {
				marked = "*" // astrisk
			}
			fmt.Fprintf(&chunkBuilder, "%s%d: %s", marked, i+1, s.Lines[i])
			if i != endIdx-1 {
				fmt.Fprintf(&chunkBuilder, "\n")
			}
		}
		if startIdx == 0 && endIdx == 0 {
			chunkBuilder.WriteString("[Empty File]")
		}
	}

	// 3. Format total active system instructions matching your state constraints
	var lockStatus string
	isLocked := false
	if s.LockStart > 0 && s.LockEnd > 0 {

		isLocked = true
		lockStatus = fmt.Sprintf("LOCKED FOR EDIT: Lines %d-%d (use CANCEL to return to READ mode) ", s.LockStart, s.LockEnd)
	} else {
		lockStatus = "READ-ONLY NAVIGATION MODE (use LOCK <start>-<end> to target those lines, use CANCEL to return to conversation)"
	}

	normal := fmt.Sprintf(
		`Active File: %s
Status: %s
Total Lines: %d
Current Viewport Window: Lines %d to %d

--- CURRENT FILE CHUNK VISIBLE ---
%s
----------- END CHUNK ------------
Note: line numbers are included only for this view
Note: Asterisks ('*') mark LOCKed lines

### PROTOCOL INSTRUCTIONS:
- You must output exactly ONE command keyword.
- No markdown wrappers (no backticks `+"`"+`, no asterisks **). No chit-chat.
- Zero leading whitespace allowed. Follow your command with exactly ONE space.
- To begin EDIT mode, LOCK a range.
- Protocol Commands are sent directly to the chat, and interpreted by this session.

### AVAILABLE PROTOCOL COMMANDS:
SELECT_FILE <filename> - Open/switch workspace file.
NEXT_CHUNK            - Scroll view window down by %d lines.
PREV_CHUNK            - Scroll view window up by %d lines.
GOTO <string>         - Search target string and center view window around it.
LOCK <start>-<end>    - Lock target coordinates to start editing.
CANCEL                - Unlock coordinates and return to search navigation.
### There are no more PROTOCOL COMMANDS ###
`,
		s.ActiveFile,
		lockStatus,
		len(s.Lines),
		s.CurrentLine,
		endIdx,
		chunkBuilder.String(),
		s.ChunkSize,
		s.ChunkSize,
	)
	if isLocked {
		nlocked := (s.LockEnd - s.LockStart) + 1
		lockmodetext := fmt.Sprintf(
			`
You are currently in EDIT mode. All output will directly replace the targeted lines.
To return to VIEW mode, begin or end your message with the word: "CANCEL" to discard this message
The entire LOCKed targeted lines range will be replaced by your entire reply, byte-by-byte.
Currently locked: %d lines
Replying with <%d lines will remove lines. Empty lines in your reply are retained.
Replying with >%d lines will insert additional lines.
If a mistake has been made ending the edit with CANCEL (on its own line) will revert the entire edit, allowing you to fix any typos and retry.
File line numbers are added for your convienence only. Do not prepend lines. Output only the raw changed lines.

			`,
			nlocked,
			nlocked,
			nlocked,
		)
		return normal + lockmodetext
	} else {
		return normal + "\n" + "send single line (`LOCK <start>-<end>`) enter EDIT mode."
	}
	//return normal
}

// Global integer status codes for clear systemic orchestration
const (
	CodeNavigated = 200
	CodeLocked    = 201
	CodeCancelled = 202
	CodeError     = 400
)

var ErrNoFilename = fmt.Errorf("no active file yet, use %q first", OpFilename)

func (s *Session) Peek(content string) error {
	switch strings.Fields(content)[0] {
	case "SELECT_FILE":
	case "NEXT_CHUNK":
	case "PREV_CHUNK":
	case "GOTO":
	case "LOCK":
	case "CANCEL":
	default:
		return fmt.Errorf("unknown operator keyword. did you mean to send a message? to exit file editor, use plain CANCEL")
	}
	return nil
}

func (s *Session) Parse(content string) ([]Action, error) {
	return OutputParser(content)
}

func (s *Session) Refresh() Result {
	// Read file safely into memory snapshot
	data, err := os.ReadFile(s.ActiveFile)
	if err != nil {
		return Result{E: fmt.Errorf("failed to open file: %w", err), Code: CodeError}
	}

	// Normalize newlines and preserve perfect line-by-line sizing
	s.Lines = strings.Split(string(data), "\n")
	s.CurrentLine = 1 // Reset viewport tracking
	s.LockStart, s.LockEnd = 0, 0
	if s.ChunkSize == 0 {
		s.ChunkSize = DefaultChunkSize
	}
	return Result{}
}

// RunAction implements [ActionRunner]
func (s *Session) RunAction(action Action) Result {
	if s.ActiveFile == "" && action.Op != OpFilename && action.Op != OpCancel {
		return Result{E: ErrNoFilename}
	}

	switch action.Op {
	case OpFilename:
		if s.ActiveFile == action.Content {
			return Result{Message: "The file was already selected. Use CANCEL to exit the file editor, or LOCK to lock existing lines for targeting.."}
		}
		s.ActiveFile = action.Content
		if r := s.Refresh(); r.E != nil {
			return r
		}
		s.LockStart = 0
		s.LockEnd = 0
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
		s.CurrentLine = max(targetLine-(s.ChunkSize/2), 1)
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
			if err != nil {
				return Result{E: errors.New("invalid coordinate format: " + err.Error()), Code: CodeError}
			}
			end, err = strconv.Atoi(parts[1])
			if err != nil {
				return Result{E: errors.New("invalid coordinate format: " + err.Error()), Code: CodeError}
			}
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
