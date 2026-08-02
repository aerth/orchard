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

// TestMain load test env
func TestMain(m *testing.M) {
	_ = godotenv.Load()
	// tools must be initialized (read json spec from embedded fs, apply Exec funcs)
	tools, err := builtins.LoadBuiltinTools(map[string]otool.ExecFunc{
		"read_file": implv1.ReadFile,
	})
	if err != nil {
		log.Fatal(err)
	}
	testtools = tools
	os.Exit(m.Run())
}

// TestBasicUsage does not call tools
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

// TestPrimitive does not call tools
func TestPrimitive(t *testing.T) {
	prompt := "summarize ./Makefile"
	stack := chatmessages.A{
		*chatmessages.NewFromRole("Chief", "user", prompt),
	}
	threadID := uuid.UUID{}
	err := chat.Chat(t.Context(), threadID, stack, chat.Ext{
		Tools:    testtools.ToMarshaler(),
		ThreadID: threadID,
		Handlers: chat.Handlers{},
	})
	if err != nil {
		t.Fatal(err)
	}
	//_ = got
}
