package implv2_test

import (
	"log"
	"log/slog"
	"os"
	"strings"
	"testing"

	"m/cmdpkg"
	"m/pkg/builtins/implv2"
	"m/pkg/chatmessages"
	"m/pkg/llm"

	"github.com/joho/godotenv"
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

func TestMain(t *testing.M) {
	cmdpkg.InitLog()
	env := ".env"
	if _, err := os.Stat(env); err != nil {
		env = "../../../.env"
	}
	if _, err := os.Stat(env); err != nil {
		log.Fatalf("env file: %v", err)
	}
	err := godotenv.Load(env)
	if err != nil {
		log.Fatalf("env: %v", err)
	}
	code := t.Run()
	os.Exit(code)
}

func TestAi(t *testing.T) {
	creq := llm.NewChatRequest()

	creq.Messages = []llm.Message{
		{Role: "system", Content: sysmsg},
		{Role: "user", Content: "edit the file: Fix typos in foo.txt"},
	}
	session := implv2.Session{}
	err := os.WriteFile("foo.txt", []byte("Hello\nw0rld\nThs is a testt.\n\nIt works\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile("foo.txt"); err != nil {
		t.Fatal(err)
	}
	for {
		if creq.Messages[len(creq.Messages)-1].Role == "assistant" {
			println("[test] test is done")
			break
		}
		generated := chatmessages.NewFromRole("", "assistant", "")
		for i := range creq.Messages[:len(creq.Messages)-1] {
			if creq.Messages[i].Role == "system" && strings.HasPrefix(creq.Messages[i].Content, "Operation successful.\n") {
				creq.Messages[i].Content = "Operation Successful"
			}
		}
		msgs := creq.Messages
		err = llm.New().Chat(t.Context(), creq, generated.GatherChatResponse)
		creq.Messages = msgs
		if err != nil {
			t.Fatal(err)
		}
		println(generated.Content)
		creq.Messages = append(creq.Messages, generated.Message)
		actions, err := session.Parse(generated.Content)
		if err != nil && session.LockEnd != 0 {
			err = session.ApplySurgicalEdit(strings.Split(generated.Content, "\n"))
			if err == nil {
				if session.ActiveFile != "foo.txt" {
					t.Fatal("only foo.txt")
				}
				err = os.WriteFile(session.ActiveFile, []byte(strings.Join(session.Lines, "\n")), 0o600)
			}
			session.LockStart = 0
			session.LockEnd = 0
		}
		if err != nil {
			t.Fatal(err)
		}
		for i := range actions {
			slog.Info("got action", "i", i, "s", actions[i].Op, "x", strings.Split(actions[i].Content, "\n")[0])
			res := session.RunAction(actions[i])
			if res.E != nil {
				t.Fatalf("error: %+v", err)
			}
			slog.Info("result", "out", res.Code)
			sys := session.SystemMessage()
			println(sys)
			creq.Messages = append(creq.Messages, llm.Message{
				Role:    "system",
				Content: "Operation successful.\n" + sys,
			})
		}
	}
}
