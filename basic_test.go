package orchard_test

import (
	"os"
	"testing"

	"m/pkg/chat"
	"m/pkg/chatmessages"

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
	err := chat.Chat(t.Context(), threadID, stack, chat.Ext{})
	if err != nil {
		t.Fatal(err)
	}
	//_ = got
}
