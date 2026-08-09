package implv2

import (
	"os"
	"strings"
	"testing"

	"m/pkg/chat"
	"m/pkg/debugging"
	"m/pkg/types/chatmessages"

	"github.com/google/uuid"
)

func TestDiff(t *testing.T) {
	before := `

def greeting():
	print("hello world")

`
	after := `

def greeting():
	print("Hello, World!")

`

	diff, err := Differ(uuid.New(), before, after)
	println(diff)
	if err != nil {
		t.Fatal(err)
	}
	threadID := uuid.New()
	convo := chatmessages.A{
		*chatmessages.NewFromRole("", "user", "Hi"),
		*chatmessages.NewFromRole("", "assistant", "Hi, how can I help you?"),
		*chatmessages.NewFromRole("", "user", "Capitalize the words in the function"),
	}.WithThreadID(threadID)
	cl := debugging.NewConsoleLogger(os.Stderr)
	got, err := DoReview(
		t.Context(), PreReview{
			History:  convo,
			Work:     nil,
			Before:   strings.Split(before, "\n"),
			After:    strings.Split(after, "\n"),
			Filename: "/tmp/non124.txt",
		}, chat.Ext{
			Handlers: []chat.ChatHandler{cl.HandleChatResponse},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	println("GOT REVIEW")
	println(got)
}
