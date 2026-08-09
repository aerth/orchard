package implv2

import (
	"testing"

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
	DoReview(t.Context(), PreReview{History: convo}, e chat.Ext)
}
