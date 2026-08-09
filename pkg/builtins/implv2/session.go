package implv2

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/aerth/mostly/stackerr"
)

const HowTo = `

## Examples for using the Edit Tool

--------- example -------------
4. The sun rises
5. Over the hills
6. Casting a light
7. Across the valley
8. To start today
--------- example -------------

### Enter line-replacement mode
Lets change line 7's "valley" to "village"
	LOCK 7
	(wait for lock verification)
	Across the village

After the replacement message is sent, the change is made in our in-memory session, syntax will be checked and the file will be updated on the disk.

### If the intention is to delete a single line, a single round without LOCK:

	DELETE_LINES 4

### If the intention is to delete a line range, a single round without LOCK:

	DELETE_LINES 4-5

### Moving the viewport

In a large file, we can use GOTO to center the viewport at a line number, or the next occurance of a string.
For example, an example session may use GOTO to navigate line numbers:
	GOTO 40
	LOCK 45
	def greet():
		hello world
	GOTO 90
	DELETE_LINES 94
Or, use GOTO to find a string:
	GOTO greet() // this goes to the next occurance of "greet"
	GOTO greet() // if there is no next occurance, it will wrap the document and return the first.
	




### Inserting text *before* or *after* another line requires two rounds:

Given a file source-view such as:

--------- example -------------
4. The sun rises
5. Over the hills
6. Casting a light
7. Across the valley
8. To start today
--------- example -------------

	1. Send the "LOCK 6-6" operation first
	2. Wait for verification, then send a raw reply with two lines: 

` + "```" + `
Casting a light
A golden glow
` + "```" + `

The source-view would become:

--------- example -------------
4. The sun rises
5. Over the hills
6. Casting a light
7. A golden glow
8. Across the valley
9. To start today
--------- example -------------

# Instructions

**Greedy Iterative Mandate:**
When tasked with repetitive file modifications (e.g., removing all empty lines, replacing a common word, or refactoring), **do not ask for confirmation or seek clarification on the "next step."** You must execute an immediate, iterative loop using coordinate-based commands:
1. **Scan**: Identify the line numbers of the targets in the current viewport.
2. **Execute**: Issue a single command to resolve the first identified target(s).
3. **Repeat**: Upon successful modification, the viewport is updated and repeat until no target patterns remain.

**Prioritize Protocol Execution over Meta-Conversation.** If the path forward is obvious via the protocol commands, execute it without discussion.

`

type Session struct {
	editor      *EditorTool
	ActiveFile  string   // Selected filename
	Lines       []string // The cached file content split by line
	oldlines    []string // pre edit
	CurrentLine int      // The top line number currently visible in the chunk window
	ChunkSize   int      // How many lines to send in a chunk (e.g., 50)

	// Lock bounds (1-based line numbers)
	LockStart int
	LockEnd   int // if set, is in line-replacement mode

	Intent string
}

func (s *Session) GetEditor() *EditorTool {
	return s.editor
}

func (s Session) CurrentView() string {
	if s.CurrentLine < 1 {
		return "(no current view)"
	}
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
				marked = "*" // astrisk lock line indicator
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
	return chunkBuilder.String()
}

const DefaultChunkSize = 75

func (s Session) Current() []byte {
	return []byte(strings.Join(s.Lines, "\n"))
}

var ErrNot = errors.New("not an operator keyword and no LOCK range selected")

func (s Session) SystemMessage() string {
	isLocked := s.LockStart > 0 && s.LockEnd > s.LockStart
	if isLocked {
		return s.SystemMessageForEdit()
	}
	return s.SystemMessageForView()
}

func (s Session) SystemMessageForEdit() string {
	normal := s.SystemMessageForBoth()
	nlocked := s.NumLinesLocked()
	lockmodetext := fmt.Sprintf(
		`

### ONLY ONE AVAILABLE PROTOCOL COMMAND

CANCEL                - Unlock target coordinates, returning to normal mode.
		
	(%d targeted lines, we are in line replacement mode):

### Line Replacement Instructions
All output will directly replace the targeted (locked) lines.
The entire targeted lines range will be replaced by your entire raw reply, byte-by-byte, without string-replacement of any kind.
Currently locked: %d lines
Replying with <%d lines will remove lines. Empty lines in your reply are retained.
Replying with >%d lines will insert additional lines.
File line numbers are added for your convienence only. Do not prepend lines. Output only the raw changed lines.
Any lines that were not targeted by lock will remain unmodified.
Best practices:
		Be quick. Do one thing in this round.
		Lock only the lines you will modify.
		Use CANCEL to ask the user a question.
		Make a single change. You may return to line replacement mode after the edit was successfully applied.
To return to NORMAL mode, begin or end your message with the word: "CANCEL".
If you meant to delete these locked lines, use CANCEL to go back, then DELETE_LINES protocol command from normal mode.

			`, nlocked,
		nlocked,
		nlocked,
		nlocked,
	)
	return normal + "\n\n" + lockmodetext
	//return normal
}

func (s Session) SystemMessageForView() string {
	return s.SystemMessageForBoth() + "\n" + ViewOnlyChunk
}

func (s Session) SystemMessageForBoth() string {
	// 1. Calculate boundaries for the active viewing chunk window

	viewportview := s.CurrentView()
	// 3. Format total active system instructions matching your state constraints
	var lockStatus string
	//isLocked := false
	if s.LockStart > 0 && s.LockEnd > 0 {
		//	isLocked = true
		lockStatus = fmt.Sprintf("%d TARGETED LINES LOCKED FOR EDIT: Lines %d-%d (use CANCEL to return to READ mode) ", s.NumLinesLocked(), s.LockStart, s.LockEnd)
	} else {
		lockStatus = "NORMAL MODE (ALL OPERATORS AVAILABLE. Use LOCK <start>-<end> to target those lines, use CANCEL to return to conversation)"
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
- To return to the conversation to ask user a question, use CANCEL.
- Zero leading whitespace allowed. Follow your command with exactly ONE space.
- Protocol Commands are sent directly to the chat, and interpreted by this session.


### There are no undocumented protocol commands ###
`,
		s.ActiveFile,
		lockStatus,
		len(s.Lines),
		s.CurrentLine,
		min(s.CurrentLine+s.ChunkSize, len(s.Lines)),
		viewportview,
	)
	return normal + HowTo
}

var ViewOnlyChunk = `- To begin replacing lines, find the line number(s) and LOCK a range to target replacement.

### AVAILABLE PROTOCOL COMMANDS NOW (normal mode, no targeted lines locked):
SELECT_FILE <filename> - Open/switch workspace file.
NEXT_CHUNK            - Scroll source-view window down by ` + fmt.Sprint(DefaultChunkSize) + ` lines.
PREV_CHUNK            - Scroll source-view window up by ` + fmt.Sprint(DefaultChunkSize) + ` lines.
GOTO <string>         - Search target string and center source-view window around it.
LOCK <start>-<end>    - Lock target coordinates to begin line-replacement. May be a single (LOCK 4)
DELETE_LINES <start>-<end>   - Delete the specified line range immediately. May be a single (DELETE_LINES 4)
CANCEL                - Return to conversation with user.
`

// Global integer status codes for clear systemic orchestration
// TODO: remove or make useful
const (
	CodeNavigated CodeInt = 200
	CodeLocked    CodeInt = 201
	CodeCancelled CodeInt = 202
	CodeError     CodeInt = 400
)

func (c CodeInt) String() string {
	switch c {
	case CodeNavigated:
		return "navigated"
	case CodeLocked:
		return "locked"
	case CodeCancelled:
		return "cancelled"
	case CodeError:
		return "errored"
	case 0:
		return "unset"
	default:
		return fmt.Sprintf("unknown: %d", int(c))
	}
}

var ErrNoFilename = fmt.Errorf("no active file yet, use %q first", OpFilename)

func (s *Session) Peek(content string) error {
	first := OP(strings.Fields(content)[0])
	if first == "" {
		return fmt.Errorf("empty peek")
	}
	if slices.Contains(AllOperations, first) {
		return nil
	}
	return fmt.Errorf("unknown operator keyword %q. did you mean to send a message? to exit file editor, use plain CANCEL", first)
}

func (s *Session) Parse(content string) ([]Action, error) {
	acts, err := OutputParser(content)
	if len(acts) > 1 {
		slog.Info("found multiple actions", "n", len(acts))
	}
	return acts, err
}

func (s *Session) Refresh() Result {
	// Read file safely into memory snapshot
	data, err := os.ReadFile(s.ActiveFile)
	if errors.Is(err, os.ErrNotExist) {
		err = nil // will create later
	}
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

func (s *Session) PromptFileHasLines() string {
	return fmt.Sprintf("File currently has %d lines.", len(s.Lines))
}

func (s *Session) PromptEdited() string {
	return fmt.Sprintf("Successfully replaced %d lines.", s.NumLinesLocked())
}

func (s *Session) NumLinesLocked() int {
	if s.LockEnd == 0 {
		return 0
	}
	return 1 + (s.LockEnd - s.LockStart)
}

const always = "Use CANCEL to exit the file editor, or use LOCK <from>-<to> to lock existing lines for targeting."

// ApplyAction implements [ActionRunner] for [RunAction] in [EditorTool.EditTool]
//
//	conversational filler, direct answers only, keep it real, no funky business
func (s *Session) ApplyAction(action Action) Result {
	if s.CurrentLine < 1 {
		s.CurrentLine = 1
	}
	if s.ActiveFile == "" && action.Op != OpFilename && action.Op != OpCancel {
		return Result{E: fmt.Errorf("no active file selected, cant %s yet. use %s first", action.Op, OpFilename), Code: CodeError}
	}

	switch action.Op {
	default:
		return Result{E: stackerr.Errorf("unhandled protocol command %q", action.Op), Code: CodeError}

	case OpCancel:
		s.LockStart, s.LockEnd = 0, 0
		return Result{Code: CodeCancelled}
	case OpFilename:
		if s.ActiveFile == action.Content {
			return Result{Message: "The file was already selected."}
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
			return Result{E: fmt.Errorf("no active file selected, use %s first", OpFilename), Code: CodeError}
		}
		if s.CurrentLine+s.ChunkSize <= len(s.Lines) {
			s.CurrentLine += s.ChunkSize
		}
		return Result{Code: CodeNavigated}

	case OpPrevChunk:
		if s.ActiveFile == "" {
			return Result{E: fmt.Errorf("no active file selected, use %s first", OpFilename), Code: CodeError}
		}
		s.CurrentLine -= s.ChunkSize
		if s.CurrentLine < 1 {
			s.CurrentLine = 1
		}
		return Result{Code: CodeNavigated}

	case OpGoTo:
		if s.ActiveFile == "" {
			return Result{E: fmt.Errorf("no active file selected, use %s first", OpFilename), Code: CodeError}
		}
		targetLine := -1
		if n, err := strconv.Atoi(action.Content); err == nil && n > 1 {
			targetLine = n
		}
		// Search the lines slice for the NEXT exact matching code text
		ext := ""
		for idx, line := range s.Lines[s.CurrentLine:] { // get NEXT
			if strings.Contains(line, action.Content) {
				targetLine = idx + 1 // Convert to 1-based indexing
				ext = " around " + action.Content
				break
			}
		}
		// If no matches, wrap file.
		if targetLine == -1 {
			for idx, line := range s.Lines[:s.CurrentLine] {
				if strings.Contains(line, action.Content) {
					targetLine = idx + 1 // Convert to 1-based indexing
					ext = " around " + action.Content
					break
				}
			}
		}
		if targetLine == -1 {
			if strings.Contains(s.CurrentLineString(), action.Content) {
				return Result{E: fmt.Errorf("viewport is already centered on the only occurance of %q, %s", action.Content, always), Code: CodeError}
			}
			return Result{E: fmt.Errorf("search target %q not found", action.Content), Code: CodeError}
		}

		// Shift viewport to center around the found line target
		newline := max(targetLine-(s.ChunkSize/2), 1)
		if s.CurrentLine == newline {
			return Result{Code: CodeNavigated, Message: fmt.Sprintf("We have already centered the viewport at line %d. %s", newline, always)}
		}
		s.CurrentLine = newline
		return Result{Code: CodeNavigated, Message: fmt.Sprintf("The viewport is now centered at line %d%s. %s", newline, ext, always)}

	case OpLockIn:
		start, end, err := ParseRange(s, action.Content)
		if err != nil {
			return Result{E: err, Code: CodeError}
		}
		s.LockStart = start
		s.LockEnd = end
		return Result{Code: CodeLocked, Message: fmt.Sprintf("Lines %d-%d are locked and line replacement mode is active. Create a raw reply output and it will replace these %d lines. Use CANCEL to return to viewing/conversation", s.LockStart, s.LockEnd, s.NumLinesLocked())}
	}
}

func (s *Session) CurrentLineString() string {
	if s.CurrentLine == 0 {
		return "" // should not happen
	}
	return s.Lines[s.CurrentLine]
}

var MaxLinesLocked = 6
