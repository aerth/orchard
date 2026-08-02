package orchard_test

import (
	"log"
	"os"
	"testing"

	"m/cmdpkg"
	"m/pkg/builtins"
	"m/pkg/builtins/implv1"
	"m/pkg/chat"
	"m/pkg/chatmessages"
	"m/pkg/otool"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

var testtools otool.Tools

func TestMain(m *testing.M) {
	tools, err := builtins.LoadBuiltinTools(map[string]otool.ExecFunc{
		"read_file": implv1.ReadFile,
	})
	if err != nil {
		log.Fatal(err)
	}
	testtools = tools
	_ = godotenv.Load()
	os.Exit(m.Run())
}

func TestBasicUsage(t *testing.T) {
	prompt := "summarize ./Makefile"
	stack := chatmessages.A{
		*chatmessages.NewFromRole("Chief", "user", prompt),
	}
	threadID := uuid.UUID{}
	err := cmdpkg.ChatWith(t.Context(), stack, testtools, chat.Ext{
		ThreadID: threadID,
		Handlers: chat.Handlers{},
	})
	if err != nil {
		t.Fatal(err)
	}
	//_ = got
}

func TestPrimitive(t *testing.T) {
}
