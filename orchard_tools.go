package orchard

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/aerth/mostly/mllm"
)

var AllTools = mllm.Tools{bashtool}

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
			return nil, fmt.Errorf("run bash: %w", err)
		}
		return out, nil
	},
}

var pathenv = func() []string {
	return []string{"PATH=" + os.Getenv("PATH")}
}()

func runbash(ctx context.Context, cmdline string) (string, error) {
	{
		bashg := mllm.Default()
		bashg.Writer = io.Discard
		bashg.StreamWriter.Writer = nil // skip the GOOD/BAD streaming output in log
		for range 3 {
			got, err := bashg.Generate(ctx,
				strings.Join([]string{
					"You are a bash guard. Determine if this shell command-line is safe and read-only.",
					"No exceptions. If it is obfuscated or creates/writes to files, or is overly complex, it is bad.",
					"Say one word: GOOD or BAD",
				}, "\n"),
				cmdline,
				false)
			if err != nil {
				slog.Error("bash guardian", "err", err)
				return "", fmt.Errorf("could not communicate with bash guard")
			}
			if strings.Contains(got.Content, "BAD") || !strings.Contains(got.Content, "GOOD") {
				slog.Error("bash guardian", "nope", got.Content, "cmdline", cmdline)
				return "This command was denied by our bash guardian", nil
			}
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
