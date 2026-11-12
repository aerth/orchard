package orchard

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/mostly/stackerr"
	"github.com/aerth/orchard/pkg/builtintools"
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
	ToolRunnerFunc func(ctx context.Context, cfg Config, caller mllm.ChatMessage, index int) (mllm.ChatMessages, error)
	GeneratorFunc  func(ctx context.Context, cfg Config, convo mllm.ChatMessages) (mllm.ChatMessage, error)
	PromptPrefix   string
	PromptSuffix   string
}{
	ToolRunnerFunc: odefaults{}.ToolRunner,
	GeneratorFunc:  odefaults{}.Generator,
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

	flag.IntVar(&defaults.LimitHistory, "limit-history", defaults.LimitHistory, "limit history (convo) given to llm")
	flag.IntVar(&defaults.LimitRounds, "limit-rounds", defaults.LimitRounds, "limit rounds (turns) in the loop (-1 infinite)")

	flag.BoolVar(&mllm.HideThink, "hide-think", mllm.HideThink, "hide thinking when thinking is enabled")
	flag.BoolVar(&mllm.HideTools, "hide-tools", mllm.HideTools, "hide tools, not recommended. prefer redirecting stderr")

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
