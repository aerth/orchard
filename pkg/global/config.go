package global

import (
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type Conf struct {
	i ConfigInput
}

func (c Conf) Get() ConfigInput {
	return c.i
}

func (c *Conf) GetUsername() string {
	return c.i.Username
}

var version string

func SetVersion(s string) {
	version = s
}

var buildtime time.Time

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
	Username    string
	Treedir     string
	Tooldir     string
	Logfilename string
	ThreadID    uuid.UUID
	Dir         string
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
	flag.StringVar(&cfg.Treedir, "tree", cfg.Treedir, "tree for markdown files for system messages (employee descriptions and task templates)")
	flag.StringVar(&cfg.Tooldir, "tool", cfg.Tooldir, "dir for python/bash tools (should contain toolbox directories of python and bash scripts)")
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

func DefaultConfigInput() ConfigInput {
	wd, err := os.Getwd()
	if err != nil {
		panic(err.Error())
	}
	threadID := uuid.Nil // nostore
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
		logfilename = filepath.Join(wd, "../orchard.log")
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
