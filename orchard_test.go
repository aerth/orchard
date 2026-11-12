package orchard_test

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"os"
	"strings"
	"testing"
	"uuid"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/orchard"
	"github.com/aerth/orchard/pkg/toolboxes"
)

type (
	ToolRunnerFunc = func(ctx context.Context, cfg orchard.Config, caller mllm.ChatMessage, index int) (mllm.ChatMessages, error)
	GeneratorFunc  = func(ctx context.Context, cfg orchard.Config, convo mllm.ChatMessages) (mllm.ChatMessage, error)
)

// duplicated for vim etc
var (
	_ ToolRunnerFunc = orchard.Custom.ToolRunnerFunc
	_ GeneratorFunc  = orchard.Custom.GeneratorFunc
)

func TestMain(m *testing.M) {
	_ = orchard.Config{}.SystemPromptFunc // test that we can access the unexported embedded struct from outside the package
	mllm.Flags()
	orchard.Flags(nil)
	if !flag.Parsed() {
		flag.Parse()
	}
	os.Exit(m.Run())
}

func Test03(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
		return
	}
	ctx := t.Context()
	toolbox, err := toolboxes.New("testdata/testtools")
	if err != nil {
		t.Fatal(err)
	}
	cfg := orchard.Config{Toolbox: toolbox, Think: false}
	out, err := orchard.CreateConvo(ctx, cfg,
		mllm.ChatMessages{{
			Role:    "user",
			Content: "Sleep for 3 seconds",
		}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 || len(out[0].ToolCalls) != 1 {
		t.Fatal("expected one tool call")
	}
	caller := out[0]
	v := caller.ToolCalls[0]
	caller.ToolCalls = []json.RawMessage{v}
	var call mllm.Call[json.RawMessage]
	err = json.Unmarshal(v, &call)
	if err != nil {
		t.Fatal(err)
	}
	if call.Function.Name != "sleep" {
		t.Fatalf("expected sleep func to be called, got %q", call.Function.Name)
	}
	if s := strings.ReplaceAll(string(call.Function.Arguments), " ", ""); s != `{"seconds":3}` {
		println(s)
		t.Fatal("expected 3 seconds")
	}
	if len(out) != 3 { // [call,tool,text]
		t.Fatalf("expected an tool output and after-tool message got %d", len(out))
	}
	if out[1].Role != "tool" {
		t.Errorf("role should be 'tool' got %q", out[1].Role)
	}
	if out[2].Role != "assistant" {
		t.Errorf("role should be 'assistant' got %q", out[2].Role)
	}
}

func Test04(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
		return
	}
	ctx := t.Context()
	cfg := orchard.Config{Toolbox: orchard.AllTools}
	//cfg.PromptPrefix = "Always speak in pirate dialect."
	//cfg.PromptSuffix = "Always speak in pirate dialect."
	_, err := orchard.CreateConvo(ctx, cfg,
		mllm.ChatMessages{{
			Role:    "user",
			Content: "Read and understand the main module ./*.go and tell me some important things that are missing from README.md that would be most helpful to add",
		}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestO1(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
		return
	}
	threadReader := func(ctx context.Context, threadID uuid.UUID) (mllm.ChatMessages, error) {
		return mllm.ChatMessages{{
			Role:    "user",
			Content: "explain the current dir repo, what does it do? examine the main files, not the test files",
		}}, nil
	}
	tools := orchard.AllTools
	threadID := uuid.Nil()
	ctx := t.Context()
	think := "none"
	mllm.DefaultOptions.Debug = 2
	mllm.DefaultWriter = os.Stderr
	add, err := orchard.Create(ctx, orchard.Config{
		ThreadReader: threadReader, Toolbox: tools,
		Think:            think,
		SystemPromptFunc: nil,
		ThreadID:         threadID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(add) < 2 {
		t.Fatal("did not")
	}
}

func TestO2(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
		return
	}
	threadReader := func(ctx context.Context, threadID uuid.UUID) (mllm.ChatMessages, error) {
		return mllm.ChatMessages{{
			Role:    "user",
			Content: "explain the current dir repo, what does it do? examine the main files, not the test files",
		}}, nil
	}
	tooldir := "../otools/"
	toolbox, err := toolboxes.NewToolboxFS(tooldir)
	if err != nil {
		t.Fatal(err)
	}
	tools := toolbox.AllTools()
	threadID := uuid.Nil()
	ctx := t.Context()
	think := "none"
	mllm.DefaultOptions.Debug = 2
	mllm.DefaultWriter = os.Stderr
	add, err := orchard.Create(ctx, orchard.Config{
		ThreadReader: threadReader, Toolbox: tools,
		Think: think,
		// SystemPrompt: orchard.DefaultSystemPrompt,
		ThreadID: threadID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(add) < 2 {
		t.Fatal("did not")
	}
}
