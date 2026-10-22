package orchard_test

import (
	"log"
	"os"
	"testing"

	"m/orchard/cmdpkg"
	"m/orchard/pkg/builtins"
	"m/orchard/pkg/builtins/implv1"
	"m/orchard/pkg/chat"
	"m/orchard/pkg/types/chatmessages"
	"m/orchard/pkg/types/otool"

	"github.com/aerth/mostly/godotenv"
	"github.com/aerth/mostly/mllm"
	"uuid"
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

// TestBasicUsage runs tool calls
func TestBasicUsage(t *testing.T) {
	prompt := "summarize ./Makefile"
	stack := chatmessages.A{
		*chatmessages.NewFromRole("Chief", "user", prompt),
	}
	threadID := uuid.UUID{}
	// [cmdpkg.ChatWith] calls [chat.Chat]
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
	// [chat.Chat] calls underlying [llm.Client.Chat]
	err := chat.ChatPlain(t.Context(), threadID, stack, chat.Ext{
		//	Prepend:  []llm.Message{{Role: "system", Content: "Keep all replies short."}},
		Tools:    testtools.ToMarshaler(),
		ThreadID: threadID,
		Handlers: chat.Handlers{mllm.NewHandlerOut(os.Stderr, mllm.DefaultThinkPre, mllm.DefaultThinkPost)},
	})
	if err != nil {
		t.Fatal(err)
	}
	//_ = got
}
