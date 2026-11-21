// Package builtintools the run_bash tool
//
// TOOD: read image tool, and other tools that are not able to be implemented as a tool script
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
	"github.com/aerth/orchard/pkg/orchardenv"
	"github.com/aerth/orchard/prompts"
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

var bashguardprompt, _ = prompts.Get("bashguard")

func AddGuardPrompt(lines string) {
	if len(bashguardprompt) == 0 {
		panic("no bashguard prompt found")
	}
	bashguardprompt += "\n"
	bashguardprompt += lines
}

var pathenv = func() []string {
	var envs []string
	for _, key := range orchardenv.OKENVNAMES() {
		val := os.Getenv(key)
		if val != "" {
			envs = append(envs, key+"="+val)
		}
	}
	return envs
}()

// runbash is used in [BashTool] [mllm.Tool.Exec] func
func runbash(ctx context.Context, cmdline string) (string, error) {
	var guardreplies []string
	{
		bashg := LLM2()
		bashg.Writer = io.Discard
		bashg.StreamWriter.Writer = nil // skip the GOOD/BAD streaming output in log
		if len(bashguardprompt) == 0 {
			return "", fmt.Errorf("no bash guardian prompt found in this version")
		}
		for range 3 {
			got, err := bashg.Generate(ctx,
				bashguardprompt,
				cmdline,
				false)
			if err != nil {
				slog.Error("bash guardian", "err", err)
				return "", fmt.Errorf("could not communicate with bash guardian")
			}
			if strings.Contains(got.Content, "BAD") || !strings.Contains(got.Content, "GOOD") {
				slog.Error("bash guardian", "nope", got.Content, "cmdline", cmdline)
				return "Error: This command was denied by our bash guardian: " + got.Content, nil // add the reason so LLM can workaround
			}
			guardreplies = append(guardreplies, got.Content)
		}
	}
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "bash", "-c", cmdline)
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Env = pathenv
	slog.Warn("running bash", "cmdline", cmdline, "guard_replies", guardreplies)
	err := cmd.Run()
	if err != nil {
		return out.String() + "\nError: " + err.Error(), nil
	}
	return out.String(), nil
}
