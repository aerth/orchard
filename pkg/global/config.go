package global

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"
)

func Dir() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err.Error())
	}
	return wd
}

type Conf struct {
	i ConfigInput
}

func (c *Conf) GetVerbose() bool {
	return c.i.ShowRequests
}

func (c *Conf) IsHideToolOuts() bool {
	return !c.i.ShowToolOuts
}

func (c *Conf) IsHideToolCalls() bool {
	return c.i.HideToolCalls
}

func (c Conf) Get() ConfigInput {
	return c.i
}

func (c *Conf) GetUsername() string {
	return c.i.Username
}

var version string

func SetVersion(s string, mode string) {
	version = s
	omode = mode
}

var (
	omode     string
	buildtime time.Time
)

func BuildTime() time.Time {
	return buildtime
}

func SetBuildTime(s string) {
	if s == "" {
		return
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err == nil {
		buildtime = time.Unix(n, 0)
		return
	}
	if err := buildtime.UnmarshalText([]byte(s)); err != nil {
		panic(err.Error())
	}
}

func GetVersion() string {
	return version
}

func GetVersionString() string {
	if omode == "" {
		omode = "testing"
	}
	return fmt.Sprintf("Orchard %s (%s) (%s)\n", version, buildtime, omode)
}

func (c Conf) GetTreeDir() string {
	return c.i.Treedir
}

func (c Conf) GetToolDir() string {
	return c.i.Tooldir
}

func (c Conf) Getwd() string {
	return c.i.Dir
}

func (c Conf) GetLogFilename() string {
	return c.i.Logfilename
}

func (c Conf) GetThreadID() uuid.UUID {
	return c.i.ThreadID
}

func envor(s, d string) string {
	if x := os.Getenv(s); x != "" {
		return x
	}
	return d
}

type ConfigInput struct {
	Username      string
	Botname       string
	Treedir       string
	Tooldir       string
	Logfilename   string
	ThreadID      uuid.UUID
	HideToolCalls bool
	ShowToolOuts  bool
	ShowRequests  bool
	Dir           string
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
	flag.BoolVar(&cfg.ShowToolOuts, "show-toolouts", cfg.ShowToolOuts, "show tool output chunks")
	flag.StringVar(&cfg.Logfilename, "logfile", cfg.Logfilename, "path to log file")
	flag.Func("thread", "uuid", func(s string) error {
		return cfg.ThreadID.UnmarshalText([]byte(s))
	})
	flag.StringVar(&cfg.Dir, "C", cfg.Dir, "change dir after loading stuff")
	if clearfirst != nil {
		flag.BoolVar(clearfirst, "clear", *clearfirst, "clear thread from DB first")
	}
	flag.Parse()
}

var (
	TestThreadID   = uuid.MustParse("dc41d7d4-a64b-4613-9ed5-4997ec33b0ab")
	SingleThreadID = TestThreadID
)

func DefaultConfigInput() ConfigInput {
	wd, err := os.Getwd()
	if err != nil {
		panic(err.Error())
	}
	//threadID := uuid.Nil // nostore
	threadID := SingleThreadID // dummy testing
	if x := os.Getenv("ORCHARD_THREAD"); x != "" {
		if err := threadID.UnmarshalText([]byte(x)); err != nil {
			panic(err.Error())
		}
	}
	username := envor("ORCHARD_USERNAME", "Chief")
	treedir := envor("ORCHARD_TREE", filepath.Join(wd, "../otree"))
	tooldir := envor("ORCHARD_TOOLDIR", filepath.Join(wd, "../otools"))
	//
	logfilename := os.Getenv("ORCHARD_LOGFILE")
	if logfilename == "" {
		logfilename = filepath.Join("/tmp/orchard.log")
	}
	//
	treedir, _ = filepath.Abs(os.ExpandEnv(treedir))
	tooldir, _ = filepath.Abs(os.ExpandEnv(tooldir))
	dir := "."
	if d, err := filepath.Abs(dir); err == nil {
		dir = d
	}
	return ConfigInput{
		ThreadID:    threadID,
		Logfilename: logfilename,
		Username:    username,
		Dir:         dir,
		Treedir:     treedir,
		Tooldir:     tooldir,
	}
}
