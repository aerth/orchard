package orchard_test

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"uuid"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/orchard"
	"github.com/aerth/orchard/pkg/toolboxes"
)

func TestFuncCheck(t *testing.T) {
	type (
		ToolRunnerFunc = func(ctx context.Context, cfg orchard.Config, caller mllm.ChatMessage, index int) (mllm.ChatMessages, error)
		GeneratorFunc  = func(ctx context.Context, cfg orchard.Config, convo mllm.ChatMessages) (mllm.ChatMessages, mllm.ChatMessage, error)
	)
	// duplicated func signatures for vim completion, but test here that they do not change
	var x ToolRunnerFunc
	var y GeneratorFunc
	x = orchard.Custom.ToolRunnerFunc
	y = orchard.Custom.GeneratorFunc
	_, _ = x, y
}

func TestMain(m *testing.M) {
	_ = orchard.Config{}.SystemPromptFunc // test that we can access the unexported embedded struct from outside the package
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level:     slog.LevelWarn,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.SourceKey {
				src := a.Value.Any().(*slog.Source)
				a.Value = slog.StringValue(fmt.Sprintf("%s#%d (%s)", filepath.Base(src.File), src.Line, filepath.Base(src.Function)))
				return a
			}
			return a
		},
	})))
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

	var out mllm.ChatMessages
	cfg.FinishHook = func(fullconvo mllm.ChatMessages) {
		println("finish hook")
		out = slices.Clone(fullconvo)
	}
	err = cfg.Run(ctx,
		mllm.ChatMessages{{
			Role:    "user",
			Content: "Sleep for 2 seconds",
		}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 2 {
		t.Fatal("FinishHook was not called")
	}
	out = out[1:] // TODO: update this test since out is no longer the new additions
	if len(out) == 0 || len(out[0].ToolCalls) != 1 {
		for i := range out {
			log.Printf("%d: %s", i, must(t, out[i].MarshalJSON))
		}
		t.Fatalf("expected one tool call (%d out)", len(out))
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
	if s := strings.ReplaceAll(string(call.Function.Arguments), " ", ""); s != `{"seconds":2}` {
		println(s)
		t.Fatal("expected 2 seconds")
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

func must[T any](t *testing.T, fn func() (v T, err error)) T {
	v, err := fn()
	if err != nil {
		t.Fatal(err)
	}
	return v
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
	err := cfg.Run(ctx,
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
	var add mllm.ChatMessages
	e := mllm.Default()
	e.ThreadID = threadID
	err := orchard.Create(ctx, orchard.Config{
		E: e,
		FinishHook: func(x mllm.ChatMessages) {
			add = x
		},
		ThreadReader: threadReader, Toolbox: tools,
		Think:            think,
		SystemPromptFunc: nil,
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
	convo := mllm.ChatMessages{{
		Role:    "user",
		Content: "explain the current dir repo, what does it do? examine the main files, not the test files",
	}}

	tooldir := "../otools/"
	toolbox, err := toolboxes.NewToolboxFS(tooldir)
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("../otools/ not found, skipping")
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	tools := toolbox.AllTools()
	threadID := uuid.Nil()
	ctx := t.Context()
	think := "none"
	mllm.DefaultOptions.Debug = 2
	mllm.DefaultWriter = os.Stderr
	var add mllm.ChatMessages

	e := mllm.Default()
	e.ThreadID = threadID
	err = orchard.Config{
		FinishHook: func(convo mllm.ChatMessages) {
			add = convo
		},
		Toolbox: tools,
		Think:   think,
		E:       e,
	}.Run(ctx, convo)
	if err != nil {
		t.Fatal(err)
	}
	if len(add) < 2 {
		t.Fatal("did not")
	}
}

func TestEmpty(t *testing.T) {
	ctx := t.Context()
	cfg := orchard.New()
	for _, thinkVal := range []any{false, "low", nil} {
		t.Run("empty-"+fmt.Sprint(thinkVal), func(t *testing.T) {
			if err := cfg.E.ValidateConfig(false); err != nil {
				t.Skip(err)
				return
			}

			if len(cfg.Toolbox)+len(cfg.E.Tools) != 0 {
				t.Fatal("found tools")
			}
			convo := mllm.ChatMessages{
				{Role: "user", Content: "what is 3+2+1"},
			}
			// should output to stdout (i.e. not to nowhere)
			cfg.Think = thinkVal
			for range 2 {
				var capture mllm.ChatMessage
				cfg.E.Handlers = []mllm.StreamHandler{capture.HandleChat}
				err := cfg.Run(ctx, convo)
				if err != nil {
					t.Fatal("run failed:", err)
				}
				capture.Reasoning = ""
				convo = append(convo, capture)
				if true { // fun
					Flip(convo)
					cfg.Toolbox = nil
					cfg.SystemPromptFunc = func(sc orchard.StateConfig) (string, error) {
						return "", nil
					}
				}
			}
		})
	}
}

func Flip(a mllm.ChatMessages) {
	for i := range a {
		switch a[i].Role {
		case "user":
			a[i].Role = "assistant"
		case "assistant":
			a[i].Role = "user"
		}
	}
}
