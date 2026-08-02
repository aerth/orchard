package orchard_test

import (
	"os"
	"testing"

	"m/pkg/builtins"
	"m/pkg/builtins/implv1"
	"m/pkg/chat"
	"m/pkg/chatmessages"
	"m/pkg/debugging"
	"m/pkg/llm"
	"m/pkg/otool"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

func TestMain(m *testing.M) {
	_ = godotenv.Load()
	os.Exit(m.Run())
}

func TestFoo(t *testing.T) {
	prompt := "summarize ./Makefile"
	usermsg := chatmessages.NewFromRole("Chief", "user", prompt)
	stack := chatmessages.A{*usermsg}
	threadID := uuid.UUID{}
	sysmsg := []llm.Message{{Role: "system", Content: "you are grug"}}
	tools := builtins.LoadBuiltinTools(map[string]otool.ExecFunc{
		"read_file": implv1.ReadFile,
	})
	cl := debugging.NewConsoleLogger()
	err := chat.Chat(t.Context(), threadID, stack, chat.Ext{
		Prepend: sysmsg,
		Tools:   tools,
		Handlers: chat.Handlers{
			cl.HandleChatResponse,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	//_ = got
}
