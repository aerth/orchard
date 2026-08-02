package orchard_test

import (
	"os"
	"testing"

	"m/cmdpkg"
	"m/pkg/builtins"
	"m/pkg/builtins/implv1"
	"m/pkg/chat"
	"m/pkg/chatmessages"
	"m/pkg/debugging"
	"m/pkg/otool"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

func TestMain(m *testing.M) {
	_ = godotenv.Load()
	os.Exit(m.Run())
}

func TestBasicUsage(t *testing.T) {
	tools, err := builtins.LoadBuiltinTools(map[string]otool.ExecFunc{
		"read_file": implv1.ReadFile,
	})
	prompt := "summarize ./Makefile"
	stack := chatmessages.A{
		*chatmessages.NewFromRole("Chief", "user", prompt),
	}
	threadID := uuid.UUID{}
	cl := debugging.NewConsoleLogger()
	err = cmdpkg.ChatWith(t.Context(), stack, tools, chat.Ext{
		ThreadID: threadID,
		Handlers: chat.Handlers{
			cl.HandleChatResponse,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	//_ = got
}
