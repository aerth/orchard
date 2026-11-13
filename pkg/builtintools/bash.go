package builtintools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/aerth/mostly/mllm"
)

// LLM2 may replace the default model (used by bash guardian)
var LLM2 = mllm.Default

func BashTool() mllm.Tool {
	return mllm.Tool{
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
}

var okkeys = []string{
	"XDG_CONFIG_HOME",
	"XDG_DATA_HOME",
	"XDG_CACHE_HOME",
	"XDG_RUNTIME_DIR",
	"XDG_DATA_DIRS",
	"XDG_CONFIG_DIRS",
	"PATH",
	"HOME",
	"USER",
	"PWD",
	"SHELL",
	"EDITOR",
	"VISUAL",
	"LANG",
	"TERM",
	"WAYLAND_DISPLAY",
	"DISPLAY",
	"DBUS_SESSION_BUS_ADDRESS",
	"CUDA_PATH",
	"NVCC_CCBIN",
	"PYTHONPATH",
	"NODE_ENV",
}

var bashguardprompt = strings.Join([]string{
	"You are a bash guard. Determine if this shell command-line is safe and read-only.",
	"No exceptions. If it is obfuscated or creates/writes to files, or is overly complex, it is bad.",
	"Say one word: GOOD or BAD",
}, "\n")

func AddGuardPrompt(lines string) {
	bashguardprompt += "\n"
	bashguardprompt += lines
}

var pathenv = func() []string {
	var envs []string
	for _, key := range okkeys {
		val := os.Getenv(key)
		if val != "" {
			envs = append(envs, key+"="+val)
		}
	}
	return envs
}()

func runbash(ctx context.Context, cmdline string) (string, error) {
	var guardreplies []string
	{
		bashg := LLM2()
		bashg.Writer = io.Discard
		bashg.StreamWriter.Writer = nil // skip the GOOD/BAD streaming output in log
		for range 3 {
			got, err := bashg.Generate(ctx,
				bashguardprompt,
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
			guardreplies = append(guardreplies, got.Content)
		}
	}
	slog.Warn("running bash", "cmdline", cmdline, "guard_replies", guardreplies)
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
