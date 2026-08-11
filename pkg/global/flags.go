package global

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ConfigInput struct {
	Username           string
	Botname            string
	Treedir            string
	Tooldir            string
	Logfilename        string
	ThreadID           uuid.UUID
	HideToolCalls      bool
	ShowToolOuts       bool
	ShowRequests       bool
	JsonOutput         bool
	NoColor            bool
	NoSpin             bool
	SystemPrompt       string
	AddSystemPrompt    string
	InitialUserPrompt  string
	TempDir            string // known good location (["./tmp/" or /tmp/foo or, 'none' for no tempdir.])
	Dir                string // the Working Directory
	ShowThinking       bool
	SidebarRefreshRate time.Duration
	StreamRefreshRate  time.Duration
	ToolsLimited       []string
}
type cfginput = ConfigInput

func NewConf(in ConfigInput) *Conf {
	input := cfginput(in)
	return &Conf{i: input}
}

func NewConfigFlags(clearfirst *bool) *Conf {
	cfg := DefaultConfigInput() // can be customized in main package
	cfg.Flags(clearfirst)       // will quit in case of -h or error
	return NewConf(cfg)
}

func (cfg *ConfigInput) Flags(clearfirst *bool) {
	threadIDSet := false // explicitly set from flag
	flag.StringVar(&cfg.Username, "username", cfg.Username, "username like Bob or Chief")
	flag.StringVar(&cfg.Botname, "bot", cfg.Botname, "username like Alice or project_manager")
	flag.StringVar(&cfg.Treedir, "tree", cfg.Treedir, "tree for markdown files for system messages (employee descriptions and task templates)")
	flag.StringVar(&cfg.Tooldir, "tool", cfg.Tooldir, "dir for python/bash tools (should contain toolbox directories of python and bash scripts)")
	flag.BoolVar(&cfg.HideToolCalls, "q", cfg.HideToolCalls, "hide tool call json chunks")
	flag.BoolVar(&cfg.ShowRequests, "v", cfg.ShowRequests, "show requests in log")
	flag.BoolVar(&cfg.ShowToolOuts, "show-toolouts", cfg.ShowToolOuts, "show tool output chunks")
	flag.StringVar(&cfg.SystemPrompt, "system", cfg.SystemPrompt, "a string or path to filename")
	flag.StringVar(&cfg.AddSystemPrompt, "addsystem", cfg.AddSystemPrompt, "a string or path to filename")
	flag.StringVar(&cfg.Logfilename, "logfile", cfg.Logfilename, "path to log file")
	flag.BoolVar(&cfg.JsonOutput, "json", cfg.JsonOutput, "json output")
	flag.StringVar(&extraReviewText, "extra-review-text", extraReviewText, "extra review text for the basher tool")
	flag.BoolVar(&cfg.NoColor, "nocolor", cfg.NoColor, "disable colorized output")
	flag.BoolVar(&cfg.ShowThinking, "show-thinking", cfg.ShowThinking, "show thinking")
	flag.BoolVar(&cfg.NoSpin, "nospin", cfg.NoSpin, "disable spin")
	flag.StringVar(&cfg.InitialUserPrompt, "placeholder", cfg.InitialUserPrompt, "the empty text area placeholder")
	flag.Func("tools", "list of tools available in this session (or 'none' for no tools)", func(s string) error {
		ss := strings.Fields(s)
		if len(ss) == 1 {
			ss = strings.Split(ss[0], ",")
		}
	})
	flag.DurationVar(&cfg.StreamRefreshRate, "stream-rate", cfg.StreamRefreshRate, "rate of stream refresh (UI)")
	flag.BoolFunc("help-full", "Show extended help including environment variables", func(s string) error {
		fmt.Println("\nEnvironment Variables:")
		fmt.Println("  ORCHARD_TREE     Path to the orchestration tree directory.")
		fmt.Println("  ORCHARD_TOOLS    Path to the tools library.")
		fmt.Println("  OLLAMA_MODEL     Model name for LLM requests.")
		fmt.Println("  POSTGRES_DSN     Connection string for PostgreSQL database.")
		os.Exit(1)
		panic("unreachable")
	})
	flag.DurationVar(&cfg.SidebarRefreshRate, "sidebar-rate", cfg.SidebarRefreshRate, "rate of sidebar refresh (UI)")
	flag.Func("thread", "uuid", func(s string) error {
		if s == "test" {
			cfg.ThreadID = SingleThreadID
		}
		if strings.Count(s, "-") != 4 { // is a word
			hashed := sha256.Sum256([]byte(s)) // 32
			copy(cfg.ThreadID[:], hashed[:])   // 16
			threadIDSet = true
			return nil
		}
		threadIDSet = true
		return cfg.ThreadID.UnmarshalText([]byte(s))
	})
	flag.StringVar(&cfg.Dir, "C", cfg.Dir, "change dir after loading stuff")
	flag.StringVar(&cfg.TempDir, "tempdir", cfg.TempDir, "temporary scratch dir that is available in system prompt template")
	if clearfirst != nil {
		flag.BoolVar(clearfirst, "clear", *clearfirst, "clear thread from DB first")
	}
	flag.Parse()
	wd, _ := os.Getwd()
	if cfg.Dir != "." && cfg.Dir != "" && cfg.Dir != wd {
		if err := os.Chdir(cfg.Dir); err != nil {
			log.Fatalf("Fatal: %v", err)
		}
		slog.Info("now in dir", "dir", cfg.Dir, "from", wd)
		time.Sleep(time.Second)
	}

	if !threadIDSet && cfg.ThreadID == uuid.Nil {
		basename := filepath.Base(wd)
		hashed := sha256.Sum256([]byte(basename)) // 32
		copy(cfg.ThreadID[:], hashed[:])          // 16
		if debug2 {
			slog.Info("using thread", "thread_flag", basename, "thread", cfg.ThreadID)
		}
	}
}

var debug2 = false
