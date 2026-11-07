package orchard_test

import (
	"context"
	_ "embed"
	"flag"
	"os"
	"testing"
	"uuid"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/orchard"
)

func TestMain(m *testing.M) {
	mllm.Flags()
	orchard.Flags()
	if !flag.Parsed() {
		flag.Parse()
	}
	os.Exit(m.Run())
}

func TestO1(t *testing.T) {
	threadReader := func(ctx context.Context, threadID uuid.UUID) (mllm.ChatMessages, error) {
		return mllm.ChatMessages{{
			Role:    "user",
			Content: "explain the current dir repo, what does it do? examine the main files",
		}}, nil
	}
	tools := orchard.AllTools
	threadID := uuid.Nil()
	ctx := t.Context()
	think := "none"
	mllm.DefaultOptions.Debug = 2
	mllm.DefaultWriter = os.Stderr
	add, err := orchard.Create(ctx, threadID, orchard.Config{
		ThreadReader: threadReader, Toolbox: tools,
		Think: think, SystemPrompt: orchard.DefaultSystemPrompt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(add) < 2 {
		t.Fatal("did not")
	}
}
