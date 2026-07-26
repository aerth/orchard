package implv2

import (
	"errors"
	"strings"
)

const sysmsg = `You are an AI code-editing agent operating inside a strict state machine workspace. 
You interact with files exclusively through five explicit commands, and a CANCEL keyword to cancel writing. 

### CRITICAL RULES:
1. Output EXACTLY one command per message. 
2. Do NOT wrap your response in markdown formatting (no backticks, no asterisks).
3. Do NOT include any conversational text, explanations, greetings, or notes.
4. Leading whitespace is STRICTLY forbidden.
5. You must follow the command word with exactly ONE space and its argument.

### PROTOCOL COMMANDS:
SELECT_FILE <filename>  - Select a target file to load its initial memory snapshot.
NEXT_CHUNK             - Move the view window forward by one chunk size.
PREV_CHUNK             - Move the view window backward by one chunk size.
GOTO <search_string>   - Jump the view window to center around a literal code string match.
LOCK <start>-<end>     - Lock explicit, 1-based line numbers to prepare for a surgical edit.
CANCEL                 - Release active line locks and return to read-only pagination mode.

### CONVERSATION FLOW:
- You will start in the "Seeking" phase. Use SELECT_FILE, NEXT_CHUNK, PREV_CHUNK, or GOTO to navigate the prefixed absolute line numbers.
- Once you locate the exact target code block, issue a LOCK command (e.g., "LOCK 101-103").
- The system will respond with a confirmation lock. ONLY then may you stream raw code lines to execute the edit.

Any deviation from this formatting protocol will cause an immediate parsing execution failure.

`

func SystemInstruction() string {
	return sysmsg
}

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
	Message string
	E       error
	Code    int
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
	default:
		return nil, errors.New("invalid protocol command: " + op)
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
	}

	return actions, nil
}
