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
	Prompt  string
	FileIn  string
	FileOut string
}

func TestAi(t *testing.T) {
	creq := llm.NewChatRequest()

	creq.Messages = []llm.Message{
		{Role: "system", Content: implv2.SystemInstruction()},
		{Role: "user", Content: "edit the file: Fix typos in foo.txt. do not decrease/increase number of lines."},
	}
	session := implv2.Session{}
	err := os.WriteFile("foo.txt", []byte("Hello\nw0rld\nThs is a testt.\n\nIt works\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile("foo.txt"); err != nil {
		t.Fatal(err)
	}
	testpass := false
	for {
		if creq.Messages[len(creq.Messages)-1].Role == "assistant" {
			println("[test] test is done (latest message is assistant)")
			break
		}
		generated := chatmessages.NewFromRole("", "assistant", "")
		for i := range creq.Messages[:len(creq.Messages)-1] {
			if creq.Messages[i].Role == "system" && strings.HasPrefix(creq.Messages[i].Content, SuccessMessage+"\n") {
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
	now, _ := os.ReadFile("foo.txt")
	if string(now) != `
Hello
world
This is a test.

It works
` {
		t.Fatalf("expected fixed but got:\n%s", now)
	}
}
