package orchard_test

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"
	"text/template"
	"time"
	"uuid"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/orchard"
)

var bashtool = mllm.Tool{
	Spec: mllm.Spec(`{
  "name": "run_bash",
  "description": "general shell tool for any purpose",
  "parameters": {
    "type": "object",
    "properties": {
      "cmdline": {
        "type": "string",
        "description": "the bash command line"
      }
    },
    "required": [
      "cmdline"
    ]
  }
}`),
	Exec: func(ctx context.Context, args map[string]any) (any, error) {
		cmdline, ok := args["cmdline"].(string)
		if !ok {
			return nil, fmt.Errorf("missing cmdline string")
		}
		if strings.Contains(cmdline, "rm ") { // ...
			return nil, fmt.Errorf("invalid cmdline")
		}
		out, err := runbash(ctx, cmdline)
		if err != nil {
			return nil, err
		}
		return out, nil
	},
}

//go:embed systemprompt.md
var systemprompttemplate string

func sysprompt() (string, error) {
	wd, _ := os.Getwd()
	return executeTemplateString(systemprompttemplate, TemplateData{
		WorkingDirectory: wd,
		CurrentTime:      time.Now().Truncate(time.Second).UTC().Format(time.DateTime),
	})
}

var pathenv = func() []string {
	return []string{"PATH=" + os.Getenv("PATH")}
}()

func runbash(ctx context.Context, cmdline string) (string, error) {
	for range 3 {
		got, err := mllm.Default().Generate(ctx,
			"You are a bash guard. Determine if this shell command-line is safe and read-only. No exceptions. If it creates/writes to files, or is overly complex, it is bad. Say one word: GOOD or BAD",
			cmdline,
			false)
		if err != nil {
			return "", fmt.Errorf("%w", err)
		}
		if strings.Contains(got.Content, "BAD") || !strings.Contains(got.Content, "GOOD") {
			return "This command was denied by our bash guardian", nil
		}
	}
	slog.Warn("running bash", "cmdline", cmdline)
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "bash", "-c", cmdline)
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Env = pathenv
	err := cmd.Run()
	if err != nil {
		return out.String() + "\nError: " + err.Error(), nil
	}
	return out.String(), nil
}

func TestO1(t *testing.T) {
	threadReader := func(ctx context.Context, threadID uuid.UUID) (mllm.ChatMessages, error) {
		return mllm.ChatMessages{{
			Role:    "user",
			Content: "explain the current dir repo, what does it do? examine the main files",
		}}, nil
	}
	tools := mllm.Tools{bashtool}
	threadID := uuid.Nil()
	ctx := t.Context()
	think := "none"
	mllm.DefaultOptions.Debug = 2
	add, err := orchard.Create(ctx, threadID, orchard.Config{
		ThreadReader: threadReader, Toolbox: tools,
		Think: think, SystemPrompt: sysprompt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(add) < 2 {
		t.Fatal("did not")
	}
}

type TemplateData struct {
	WorkingDirectory string
	CurrentTime      string
}

func executeTemplateString(s string, d TemplateData) (string, error) {
	var buf bytes.Buffer
	t := template.Must(template.New("").Parse(s))
	if err := t.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}
