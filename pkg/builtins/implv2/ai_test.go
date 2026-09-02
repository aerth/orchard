package implv2_test

import (
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"

	"m/orchard/cmdpkg"
	"m/orchard/pkg/builtins/implv2"
	"m/orchard/pkg/llm"
	"m/orchard/pkg/loaders/treesitters"
	"m/orchard/pkg/types/chatmessages"

	"github.com/joho/godotenv"
)

func TestMain(t *testing.M) {
	cmdpkg.InitLog(true, " ")
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

const SuccessMessage = "Operation successful."

type TC struct {
	Filename string
	Prompt   string
	FileInp  string
	FileOut  string
}

var tcs = []TC{
	{
		Filename: "foo.txt", Prompt: "edit the file: Fix typos in foo.txt. do not decrease/increase number of lines.",
		FileInp: "Hello\nw0rld\nThs is a testt.\n\nIt works\n",
		FileOut: "Hello\nworld\nThis is a test.\n\nIt works\n",
	},
	{
		Filename: "foo.py",
		Prompt:   "edit the file: Fix the logic error in foo.py , function calculate_total to include the tax rate calculation.",
		FileInp:  "def calculate_total(price, tax_rate):\n    return price\n",
		FileOut:  "def calculate_total(price, tax_rate):\n    return price * (1 + tax_rate)\n",
	},

	{
		Filename: "foo.go",
		Prompt:   "edit the file (foo.go): Export the Add function by correcting its name casing.",
		FileInp:  "package main\n\n// Add sums two integers\nfunc add(a, b int) int {\n\treturn a + b\n}\n",
		FileOut:  "package main\n\n// Add sums two integers\nfunc Add(a, b int) int {\n\treturn a + b\n}\n",
	},
}

// TestAi is lagging behind. see tool.go
func TestAi(t *testing.T) {
	if testing.Short() {
		t.Skipf("skipping ai slow test because -short flag was found")
		return
	}
	t.Chdir(t.TempDir())
	slices.Reverse(tcs)
	for i, tc := range tcs {
		if i == 1 {
			continue
		}
		if tc.Filename == "" || tc.Prompt == "" || tc.FileInp == "" {
			t.Fatal("bad tc")
		}
		creq := llm.NewChatRequest()

		creq.Messages = []llm.Message{
			{Role: "system", Content: implv2.SystemInstruction()},
			{Role: "user", Content: tc.Prompt},
		}
		session := implv2.Session{}
		err := os.WriteFile(tc.Filename, []byte(tc.FileInp), 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.ReadFile(tc.Filename); err != nil {
			t.Fatal(err)
		}
		testpass := false
		for {
			if creq.Messages[len(creq.Messages)-1].Role == "assistant" {
				println("[test] test is done (latest message is assistant)")
				break
			}
			generated := chatmessages.NewFromRole("", "assistant", "")
			//		for i := range creq.Messages[1 : len(creq.Messages)-1] {
			//			if creq.Messages[i].Role == "system" && strings.Count(creq.Messages[i].Content, "\n") > 1 {
			//				creq.Messages[i].Content = strings.Split(creq.Messages[i].Content, "\n")[0]
			//			}
			//		}
			msgs := creq.Messages
			creq.Messages = append(creq.Messages, llm.Message{Role: "system", Content: session.SystemMessage()})
			// CHAT
			err = llm.New().Chat(t.Context(), creq, generated.GatherChatResponse)
			creq.Messages = msgs
			if err != nil {
				t.Fatal(err)
			}
			println("OUT: " + generated.Content)
			creq.Messages = append(creq.Messages, generated.Message)
			actions, err := session.Parse(generated.Content)
			if errors.Is(err, implv2.ErrNot) && session.LockEnd == 0 {
				// finished. latest was a "I have corrected the typos in `foo.txt` while maintaining the exact number of lines." message
				break
			}
			if errors.Is(err, implv2.ErrNot) && session.LockEnd != 0 {
				// target acquired
				err = session.ApplySurgicalEdit(strings.Split(generated.Content, "\n"))
				if err != nil {
					creq.Messages = append(creq.Messages, llm.Message{Role: "system", Content: fmt.Sprintf("The file edit failed: %+v", err)})
					continue
				}
				if err == nil && !strings.HasSuffix(session.ActiveFile, ".txt") {
					lang, langname, _ := treesitters.Getlang(session.ActiveFile)
					if lang != nil {
						err = implv2.ValidateFileSyntax(string(session.Current()), langname, lang)
					}
				}
				if err == nil {
					if session.ActiveFile != tc.Filename {
						t.Fatal("only one file can be edited but got " + session.ActiveFile)
					}
					err = os.WriteFile(session.ActiveFile, session.Current(), 0o600)
					testpass = err == nil
				}
				session.LockStart = 0
				session.LockEnd = 0
				creq.Messages = append(creq.Messages, llm.Message{
					Role:    "system",
					Content: "The file edits were applied to the filesystem",
				})

			}
			if err != nil {
				t.Fatalf("%+v", err)
			}
			for i := range actions {
				if actions[i].Op == implv2.OpCancel {
					slog.Info("cancel requested")
					return
				}
				slog.Info("got action", "i", i, "s", actions[i].Op, "x", strings.Split(actions[i].Content, "\n")[0])
				res := session.ApplyAction(actions[i])
				if res.E != nil {
					t.Fatalf("error: %+v", err)
				}
				slog.Info("result", "out", res.Code, "message", res.Message)
				if res.Message == "" {
					res.Message = SuccessMessage
				}
				creq.Messages = append(creq.Messages, llm.Message{
					Role:    "system",
					Content: res.Message + "\n" + session.SystemMessage(),
				})
				println(creq.Messages[len(creq.Messages)-1].Content)
			}
		}
		if !testpass {
			t.Fatal("did not pass")
		}
		now, _ := os.ReadFile(tc.Filename)

		if string(now) != tc.FileOut {
			t.Fatalf("expected fixed.want:\n%q\ngot:\n%q", tc.FileOut, now)
		}
	}
}
