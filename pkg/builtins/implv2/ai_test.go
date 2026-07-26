package implv2_test

import (
	"encoding/json"
	"errors"
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

const SuccessMessage = "Operation successful."

type TC struct {
	Filename string
	Prompt   string
	FileIn   string
	FileOut  string
}

var tcs = []TC{
	{Filename: "foo.txt", Prompt: "edit the file: Fix typos in foo.txt. do not decrease/increase number of lines.", FileIn: "Hello\nw0rld\nThs is a testt.\n\nIt works\n", FileOut: "Hello\nWorld\nThis is a test.\n\nIt works\n"},
}

func TestAi(t *testing.T) {
	for _, tc := range tcs {
		if tc.Filename == "" || tc.Prompt == "" || tc.FileIn == "" {
			t.Fatal("bad tc")
		}
		creq := llm.NewChatRequest()

		creq.Messages = []llm.Message{
			{Role: "system", Content: implv2.SystemInstruction()},
			{Role: "user", Content: tc.Prompt},
		}
		session := implv2.Session{}
		err := os.WriteFile(tc.Filename, []byte(tc.FileIn), 0o600)
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
			for i := range creq.Messages[1 : len(creq.Messages)-1] {
				if creq.Messages[i].Role == "system" && strings.Count(creq.Messages[i].Content, "\n") > 1 {
					creq.Messages[i].Content = strings.Split(creq.Messages[i].Content, "\n")[0]
				}
			}
			msgs := creq.Messages
			if testpass {
				_ = json.NewEncoder(os.Stderr).Encode(msgs)
			}
			err = llm.New().Chat(t.Context(), creq, generated.GatherChatResponse)
			creq.Messages = msgs
			if err != nil {
				t.Fatal(err)
			}
			println(generated.Content)
			creq.Messages = append(creq.Messages, generated.Message)
			actions, err := session.Parse(generated.Content)
			if errors.Is(err, implv2.ErrNot) && session.LockEnd != 0 {
					err = session.ApplySurgicalEdit(strings.Split(generated.Content, "\n"))
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
			}
			if err != nil {
				t.Fatal(err)
			}
			for i := range actions {
				if actions[i].Op == implv2.OpCancel {
					slog.Info("cancel requested")
					return
				}
				slog.Info("got action", "i", i, "s", actions[i].Op, "x", strings.Split(actions[i].Content, "\n")[0])
				res := session.RunAction(actions[i])
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
			t.Fatalf("expected fixed but got:\n%q", now)
		}
	}
}
