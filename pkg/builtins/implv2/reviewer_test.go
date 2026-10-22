package implv2

import (
	"strings"
	"testing"

	"m/orchard/pkg/builtins"
	"m/orchard/pkg/builtins/implv1"
	"m/orchard/pkg/chat"
	"m/orchard/pkg/types/chatmessages"
	"m/orchard/pkg/types/otool"

	"uuid"
)

func TestDiff(t *testing.T) {
	var reviewerTools otool.Tools
	before := `

def greeting():
	print("hello world")

`
	after := `

def greeting():
	print("Hello, World!")

`

	//	diff, err := Differ(uuid.New(), before, after)
	//	println(diff)
	//	if err != nil {
	//		t.Fatal("Differ failed:", err)
	//	}
	threadID := uuid.New()
	convo := chatmessages.A{
		*chatmessages.NewFromRole("", "user", "Hi"),
		*chatmessages.NewFromRole("", "assistant", "Hi, how can I help you?"),
		*chatmessages.NewFromRole("", "user", "Capitalize the words in the function"),
	}.WithThreadID(threadID)
	//	cl := debugging.NewConsoleLogger(os.Stderr)
	got, err := DoReview(
		t.Context(), PreReview{
			History:  convo,
			Work:     nil,
			Before:   strings.Split(before, "\n"),
			After:    strings.Split(after, "\n"),
			Filename: "/tmp/non124.txt",
		},

		reviewerTools,
		chat.Ext{
			//	Handlers: []chat.ChatHandler{cl.HandleChatResponse},
			ThreadID: uuid.New(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	println("GOT REVIEW")
	println(got)
}

func LoadBuiltinTools(t *testing.T) otool.Tools {
	builtinTools, err := builtins.LoadBuiltinTools(map[string]otool.ExecFunc{
		"read_file": implv1.ReadFile,
		//		"list_directory": implv1.ListDirectory,
		//	"edit_file": editor.EditTool,
		//"run_bash": basher1.Basher,
	})
	if err != nil {
		t.Fatalf("LoadBuiltinTools: %+v", err)
	}
	return builtinTools
}
