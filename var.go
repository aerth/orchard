package orchard

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"uuid"

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

// default has ONLY: LimitHistory, LimitRounds, Think
// if this changes, update [New]
var defaults = Config{
	LimitHistory: 20,
	LimitRounds:  -1,
	Think:        nil,
}

// GetDefaults allows modifying ONLY: LimitHistory, LimitRounds, Think
// Modifying these 3 should be done **before** calling [Flags] to set defaults,
// to avoid overriding the user command line
func GetDefaults() *Config {
	return &defaults
}

// Custom loop funcs to overhaul parts of the main orchard loop. Used by [runConvo] (and [Create])
var Custom = struct {
	// ToolRunnerFunc runs a [mllm.Tool.Exec] function, returning a message.
	// Errors end the loop. ToolRunnerFunc should usually re-insert the error into a message to allow self-healing.
	//
	// If a tool is not found by name, this should return a message or error
	ToolRunnerFunc func(ctx context.Context, cfg Config, caller mllm.ChatMessage, index int) (mllm.ChatMessages, error)
	// GeneratorFunc generates a [mllm.ChatMessage] from the convo.
	GeneratorFunc func(ctx context.Context, cfg Config, convo mllm.ChatMessages) (mllm.ChatMessages, mllm.ChatMessage, error)

	// PromptPrefix added to sticky system prompt
	PromptPrefix string
	// PromptSuffix added to sticky system prompt
	PromptSuffix string

	// PostToolHookCommands are bash commandlines that are run after matching tools are ran. '*' for all
	PostToolHookCommands map[string]string
	// PostToolHookFunc runs the bash cmd, returns convo. func might add to convo if needed.
	PostToolHookFunc func(ctx context.Context, cfg Config, convo mllm.ChatMessages, cmd string) (mllm.ChatMessages, error)
}{
	ToolRunnerFunc:       odefaults{}.ToolRunner,
	GeneratorFunc:        odefaults{}.Generator,
	PostToolHookCommands: map[string]string{},
	PostToolHookFunc:     odefaults{}.PostToolHook,
}

var (
	UseFns = false // set by application to signal deferred flag functions are supported, see [ExtendedFlags]

	Fns []func(cfg *Config) error // deferred flag functions handled by application. may modify config, see [ExtendedFlags]
)

// ExtendedFlags (see [UseFns], [Fns])
func ExtendedFlags() {
	if UseFns {
		flag.Func("delete-thread", "rename current auto-thread to new uuid/name", func(x string) error {
			if !UseFns {
				return Errorf("this flag is disabled")
			}
			curwd, err := os.Getwd() // TODO: support -C too
			if err != nil {
				return err
			}
			id := mllm.ThreadHasher(curwd)
			{ // peek --thread flag
				i := slices.IndexFunc(os.Args, func(argv string) bool {
					return argv == "-thread" || argv == "--thread"
				})
				if i >= 0 {
					parsed, err := uuid.Parse(os.Args[i+1]) // TODO: should not panic? flag parse?
					if err != nil {
						return Errorf("parsing arg %d: %v", i+1, err)
					}
					id = parsed

				}
			}
			if x != "confirm" {
				return fmt.Errorf("use --delete-thread confirm to delete thread %s", id.String())
			}
			Fns = append(Fns, func(cfg *Config) error {
				db := cfg.DB
				if db == nil {
					return Errorf("only db was nil")
				}
				n, err := db.Where("thread = ?", id).Delete(context.TODO())
				if err != nil {
					return err
				}
				if n == 0 {
					return Errorf("no matches found to update db: thread=%s", id)
				}
				slog.Info(fmt.Sprintf("database migration completed: %d records removed. restart the program", n))
				return nil
			})
			return nil
		})
		flag.Func("rename-thread", "rename current auto-thread to new uuid/name", func(newName string) error {
			if !UseFns {
				return Errorf("this flag is disabled")
			}
			curwd, err := os.Getwd()
			if err != nil {
				return err
			}
			oldID := mllm.ThreadHasher(curwd)
			{ // peek --thread flag
				i := slices.IndexFunc(os.Args, func(argv string) bool {
					return argv == "-thread" || argv == "--thread"
				})
				if i >= 0 {
					id, err := uuid.Parse(os.Args[i+1]) // TODO: should not panic? flag parse?
					if err != nil {
						return Errorf("parsing arg %d: %v", i+1, err)
					}
					oldID = id

				}
			}
			newID, _ := uuid.Parse(newName)
			if newID.String() != newName {
				newID = mllm.ThreadHasher(newName)
			}
			Fns = append(Fns, func(cfg *Config) error {
				db := cfg.DB
				if db == nil {
					return Errorf("only db was nil")
				}
				n, err := db.Where("thread = ?", oldID).Update(context.TODO(), "thread", newID)
				if err != nil {
					return err
				}
				if n == 0 {
					return Errorf("no matches found to update db: old=%s new=%s", oldID, newID)
				}

				slog.Info(fmt.Sprintf("database migration completed: %d records updated. restart the program", n))
				return nil
			})
			return nil
		})
	}
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
	flag.DurationVar(&sleepBetween, "slow", sleepBetween, "sleep between turns (default 0 disabled)")
	flag.Func("add-system", "add a file to the sticky system prompt", func(s string) error {
		b, err := os.ReadFile(s)
		if err != nil {
			return stackerr.Errorf("opening file for add-system flag: %w", err)
		}
		Custom.PromptSuffix = string(b) + "\n" + Custom.PromptSuffix
		return nil
	})

	flag.BoolVar(&mllm.HideThink, "hide-think", mllm.HideThink, "hide thinking when thinking is enabled")
	flag.IntVar(&mllm.TruncateLogToolOutChars, "hide-tools", mllm.TruncateLogToolOutChars, "for printing, truncate tool output to this many chars. negative = infinite, zero = hidden")
	flag.Func("toolhook", "add tool hook, a shell cmdline for after running tool by name. (can be used multiple times) in format toolname:commandline. use quotes. for example: --toolhook \"apply_patch_id:make lint\" or --toolhook \"all:make lint\"", func(s string) error {
		pre, post, ok := strings.Cut(s, ":")
		if !ok || pre == "" || post == "" {
			return Errorf("invalid format toolhook flag. shell quoting issue? try: --toolhook \"apply_patch_id:make lint\"")
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
