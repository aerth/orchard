package global

import (
	"crypto/sha256"
	"flag"
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	"m/cmdpkg/spinner"

	"github.com/google/uuid"
)

type ConfigInput struct {
	Username          string
	Botname           string
	Treedir           string
	Tooldir           string
	Logfilename       string
	ThreadID          uuid.UUID
	HideToolCalls     bool
	ShowToolOuts      bool
	ShowRequests      bool
	JsonOutput        bool
	NoColor           bool
	NoSpin            bool
	SystemPrompt      string
	AddSystemPrompt   string
	InitialUserPrompt string
	Dir               string
}
type cfginput = ConfigInput

func NewConf(in ConfigInput) *Conf {
	input := cfginput(in)
	return &Conf{i: input}
}

func NewConfigFlags(clearfirst *bool) *Conf {
	cfg := DefaultConfigInput()
	cfg.Flags(clearfirst) // will quit in case of -h or error
	return NewConf(cfg)
}

func (cfg *ConfigInput) Flags(clearfirst *bool) {
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
	flag.BoolVar(&cfg.NoColor, "nocolor", cfg.NoColor, "disable colorized output")
	flag.BoolVar(&cfg.NoSpin, "nospin", cfg.NoSpin, "disable spin")
	flag.StringVar(&cfg.InitialUserPrompt, "placeholder", cfg.InitialUserPrompt, "the empty text area placeholder")
	flag.Func("thread", "uuid", func(s string) error {
		if s == "test" {
			cfg.ThreadID = SingleThreadID
		}
		if strings.Count(s, "-") != 4 {

			hashed := sha256.Sum256([]byte(s)) // 32
			copy(cfg.ThreadID[:], hashed[:])   // 16
			return nil
		}
		return cfg.ThreadID.UnmarshalText([]byte(s))
	})
	flag.StringVar(&cfg.Dir, "C", cfg.Dir, "change dir after loading stuff")
	if clearfirst != nil {
		flag.BoolVar(clearfirst, "clear", *clearfirst, "clear thread from DB first")
	}
	flag.Parse()
	spinner.NoSpin = cfg.NoSpin
	wd, _ := os.Getwd()
	if cfg.Dir != "." && cfg.Dir != "" && cfg.Dir != wd {
		if err := os.Chdir(cfg.Dir); err != nil {
			log.Fatalf("Fatal: %v", err)
		}
		slog.Info("now in dir", "dir", cfg.Dir, "from", wd)
		time.Sleep(time.Second)
	}
}
