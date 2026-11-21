package orchard

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/mostly/stackerr"
	"github.com/aerth/orchard/pkg/builtintools"
	"github.com/aerth/orchard/pkg/compactor"
)

// AllTools helper is not used by orchard package, but may be used in main.
//
// By default, contains a single tool: run_bash
var AllTools = mllm.Tools{builtintools.BashTool()}

// Slog may be replaced with a custom slog logger
var Slog = mllm.Slog

// Errorf may be replaced by your preferred error wrapper
//
// By default, uses `stackerr` package for nice tracing ability
var Errorf = stackerr.Errorf

var defaults = Config{
	LimitHistory: 20,
	LimitRounds:  -1,
	Think:        nil,
}

// Custom loop funcs to overhaul parts of the main orchard loop. Used by [CreateConvo] (and [Create])
var Custom = struct {
	// ToolRunnerFunc runs a [mllm.Tool.Exec] function, returning a message.
	// Errors end the loop. ToolRunnerFunc should usually re-insert the error into a message to allow self-healing.
	ToolRunnerFunc func(ctx context.Context, cfg Config, caller mllm.ChatMessage, index int) (mllm.ChatMessages, error)
	// GeneratorFunc generates a [mllm.ChatMessage] from the convo.
	GeneratorFunc func(ctx context.Context, cfg Config, convo mllm.ChatMessages) (mllm.ChatMessage, error)

	// PromptPrefix added to sticky system prompt
	PromptPrefix string
	// PromptSuffix added to sticky system prompt
	PromptSuffix string

	// PostToolHookCommands are bash commandlines that are run after matching tools are ran. '*' for all
	PostToolHookCommands map[string]string
}{
	ToolRunnerFunc:       odefaults{}.ToolRunner,
	GeneratorFunc:        odefaults{}.Generator,
	PostToolHookCommands: map[string]string{},
}

// Flags are optional
func Flags(sloglvl *slog.LevelVar) {
	flag.Func("think", "think value, such as 'low' or 'none', this is **not checked** before sending to llm", func(s string) error {
		defaults.Think = s
		if s == "false" {
			defaults.Think = false
		}
		if s == "true" {
			defaults.Think = true
		}
		return nil
	})
	flag.IntVar(&compactor.CompactToolOutputLen, "compact-toolout-size", compactor.CompactToolOutputLen, "compact all tool outputs that are larger than n characters (0 disable)")
	flag.IntVar(&compactor.CompactConvoAt, "compact", compactor.CompactConvoAt, "auto-compact convo after it grows to n messages (0 disable)")

	flag.IntVar(&defaults.LimitHistory, "limit-history", defaults.LimitHistory, "limit history (convo) given to llm")
	flag.IntVar(&defaults.LimitRounds, "limit-rounds", defaults.LimitRounds, "limit rounds (turns) in the loop (-1 infinite)")

	flag.BoolVar(&mllm.HideThink, "hide-think", mllm.HideThink, "hide thinking when thinking is enabled")
	flag.BoolVar(&mllm.HideTools, "hide-tools", mllm.HideTools, "hide tools, not recommended. prefer redirecting stderr")
	flag.Func("toolhook", "add tool hook (can be used multiple times) in format name:command, for example: \"apply_patch_id:make lint\"", func(s string) error {
		pre, post, ok := strings.Cut(s, ":")
		if !ok || pre == "" || post == "" {
			return fmt.Errorf("invalid format toolhook flag. shell quoting issue? try: --toolhook \"apply_patch_id:make lint\"")
		}
		if pre == "all" {
			pre = "*"
		}
		Custom.PostToolHookCommands[pre] = post
		return nil
	})

	if sloglvl != nil {
		flag.Func("log-level", "debug,info,warn,error", func(s string) error {
			if err := sloglvl.UnmarshalText([]byte(s)); err != nil {
				return err
			}

			return nil
		})
	}

	flag.Func("C", "switch directory first", func(s string) error {
		st, err := os.Stat(s)
		if err != nil {
			return Errorf("directory must exist: %w", err)
		}
		if !st.IsDir() {
			return Errorf("given %q not a directory", s)
		}
		if err := os.Chdir(s); err != nil {
			return Errorf("could not change directory: %w", err)
		}
		return nil
	})
}
