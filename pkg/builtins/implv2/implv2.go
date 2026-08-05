package implv2

import (
	"errors"
	"slices"
	"strings"

	"github.com/aerth/mostly/stackerr"
)

const sysmsg = `You are an AI code-editing agent operating inside a strict state machine workspace. 
You interact with files exclusively through six explicit operator keywords. 
One action may be given.

### CRITICAL RULES:
1. Output exactly one command per message. 
2. Do NOT wrap your response in markdown formatting (no backticks, no asterisks).
3. Do NOT include any conversational text, explanations, greetings, or notes.
4. Leading whitespace is STRICTLY forbidden.
5. You must follow the command word with exactly ONE space and its argument.
6. Use the CANCEL operator to return to conversation mode.

### PROTOCOL COMMANDS:
SELECT_FILE <filename>  - Select a target file to load its initial memory snapshot.
NEXT_CHUNK             - Move the view window forward by one chunk size.
PREV_CHUNK             - Move the view window backward by one chunk size.
GOTO <search_string>   - Jump the view window to center around a literal code string match.
LOCK <start>-<end>     - Lock explicit, 1-based line numbers to prepare for a surgical edit.
CANCEL                 - Unlock the lines, returning to previous mode.

### CONVERSATION FLOW:
- You will start in the **READ** phase. Use SELECT_FILE, NEXT_CHUNK, PREV_CHUNK, or GOTO to navigate the prefixed absolute line numbers.
- Once you locate the exact target code block, issue a LOCK command (e.g., "LOCK 101-103").
- The system will respond with a confirmation lock. ONLY then may you stream raw code lines to execute the edit.

Any deviation from this formatting protocol will cause an immediate parsing execution failure.
`

func SystemInstruction() string {
	return sysmsg
}

type Action struct {
	Op      OP
	Content string
}

type OP string

const (
	OpNextChunk   OP = "NEXT_CHUNK"
	OpPrevChunk   OP = "PREV_CHUNK"
	OpGoTo        OP = "GOTO"
	OpLockIn      OP = "LOCK"
	OpCancel      OP = "CANCEL"
	OpFilename    OP = "SELECT_FILE"
	OpDeleteLines OP = "DELETE_LINES"
	// NEW
	OpJump          OP = "JUMP"       // <number>
	OpDisableLint   OP = "NOLINT"     // TODO: clarify
	OpWriteFile     OP = "WRITE_FILE" // <void>
	OpRevertChanges OP = "REVERT_ALL" // <void>
	OpUndo          OP = "UNDO"       // <void> undo single change
)

var AllOperations = []OP{OpFilename, OpGoTo, OpNextChunk, OpPrevChunk, OpDeleteLines, OpCancel, OpLockIn}

type Result struct {
	Message string
	E       error
	Code    CodeInt
}
type CodeInt int

type ActionRunner interface {
	RunAction(Action) Result
}

func RunAction(runner ActionRunner, action Action) (Result, error) {
	got := runner.RunAction(action)
	return got, got.E
}

func OutputParser(content string) ([]Action, error) {
	var op OP
	var remainder string
	var actions []Action
	{
		fields := strings.Fields(content)
		if len(fields) == 0 {
			return nil, errors.New("empty LLM response")
		}
		// 1. Strict Leading Space Check: Content must start exactly with the first field
		op = OP(fields[0])
		if !strings.HasPrefix(content, string(op)) {
			return nil, errors.New("leading whitespace is strictly forbidden")
		}
		if !slices.Contains(AllOperations, OP(op)) {
			return nil, stackerr.Errorf("editor operator not found: %q %w", content, ErrNot)
		}
		// Remove the operator prefix cleanly
		if len(content) != len(op) {
			remainder = content[len(op)+1:]
		}
	}

	switch OP(op) {
	default:
		return nil, stackerr.Errorf("editor operator not found: %q  %w", content, ErrNot)
	case OpNextChunk, OpPrevChunk, OpCancel: // no args
		if remainder != "" {
			return nil, errors.New("editor operator" + string(op) + " does not accept arguments or trailing formatting")
		}
		actions = append(actions, Action{Op: op})

	case OpGoTo, OpLockIn, OpFilename, OpDeleteLines:
		if remainder == "" {
			return nil, errors.New("editor operator " + string(op) + " must be followed by exactly one space and a target (line number or variable/function/phrase search query")
		}

		// these accept ranges which should be numbers
		if op == OpLockIn || op == OpDeleteLines {
			for i := 0; i < len(remainder); i++ {
				c := remainder[i]
				if (c < '0' || c > '9') && c != '-' {
					return nil, errors.New("LOCK arguments contain invalid characters, trailing text, or punctuation")
				}
			}
		}
		actions = append(actions, Action{Op: op, Content: remainder})
	}

	return actions, nil
}
