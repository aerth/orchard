package global

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"uuid"

	"github.com/mattn/go-isatty"
)

func ExtraReviewText() string {
	return extraReviewText
}

var extraReviewText string

func Dir() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err.Error())
	}
	return wd
}

func RefreshDir() {
	wd, _ := os.Getwd()
	gconf.i.Dir = wd
	slog.Info("now in", "dir", wd)
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

type dbstore interface {
	IsDB()
}

var gstore any

func SetStore[T dbstore](store T) {
	gstore = store
}

func GetStore[T any]() T {
	x, ok := gstore.(T)
	if !ok && gstore != nil {
		slog.Warn(fmt.Sprintf("not a %T, is %T", new(T), gstore))
	}
	return x
}

func GetVersionString() string {
	if omode == "" {
		omode = "testing"
	}
	return fmt.Sprintf("Orchard %s (%s) (%s)\n", version, buildtime.Format(time.DateOnly), omode)
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

var (
	TestThreadID   = uuid.MustParse("dc41d7d4-a64b-4613-9ed5-4997ec33b0ab")
	SingleThreadID = TestThreadID
)

var DefaultConfigInput = FreshDefaultConfigInput

func FreshDefaultConfigInput() ConfigInput {
	wd, err := os.Getwd()
	if err != nil {
		panic(err.Error())
	}
	threadID := uuid.Nil() // nostore
	//threadID := SingleThreadID // dummy testing
	if x := os.Getenv("ORCHARD_THREAD"); x != "" {
		if err := threadID.UnmarshalText([]byte(x)); err != nil {
			panic(err.Error())
		}
	}
	username := envor("ORCHARD_USERNAME", "Chief")
	botname := envor("ORCHARD_BOTNAME", "project_manager")
	treedir := envor("ORCHARD_TREE", filepath.Join(wd, "../otree"))
	tooldir := envor("ORCHARD_TOOLDIR", filepath.Join(wd, "../otools"))
	//
	logfilename := os.Getenv("ORCHARD_LOGFILE")
	if logfilename == "" {
		logfilename = filepath.Join("/tmp/orchard.log")
	}
	tmpdir := filepath.Join(os.TempDir(), "orchard-chat", "1")
	//
	treedir, _ = filepath.Abs(os.ExpandEnv(treedir))
	tooldir, _ = filepath.Abs(os.ExpandEnv(tooldir))
	dir := "."
	if d, err := filepath.Abs(dir); err == nil {
		dir = d
	}
	noColor := os.Getenv("NO_COLOR") != "" || !isatty.IsTerminal(os.Stdout.Fd())
	return ConfigInput{
		ThreadID:      threadID,
		Logfilename:   logfilename,
		Username:      username,
		Botname:       botname,
		Dir:           dir,
		TempDir:       tmpdir,
		Treedir:       treedir,
		Tooldir:       tooldir,
		NoColor:       noColor,
		HideToolCalls: false,
		ShowToolOuts:  true,
		ShowRequests:  true,
	}
}
