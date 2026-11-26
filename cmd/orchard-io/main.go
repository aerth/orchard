package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/aerth/mostly/godotenv"
	"github.com/aerth/mostly/mllm"
	"github.com/aerth/mostly/stackerr"
	"github.com/aerth/orchard"
	"github.com/aerth/orchard/pkg/compactor"
	"github.com/aerth/orchard/pkg/toolboxes"
	"github.com/aerth/orchard/pkg/visions"
	"gorm.io/gorm"
)

var client orchard.Config

var sloglvl slog.LevelVar

func main() {
	sloglvl.Set(slog.LevelDebug)
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: false,
		Level:     &sloglvl,
	})))
	prefix := ""
	suffix := ""
	dbflag := false
	//
	mllm.Flags()
	compactor.Flags()
	orchard.Flags(&sloglvl)
	//
	var allowedToolNames []string
	sleepDur := time.Second * 2
	flag.DurationVar(&sleepDur, "toolslow", sleepDur, "sleep duration after tool response is created. set 0 to make quick. or 1s 2s 3s etc")
	flag.StringVar(&prefix, "system", prefix, "for example, orchard-io -system 'explain this file, plaintext paragraph, no markdown' < README.md")
	addToolWhitelist := func(toolName string) error {
		toolName = strings.ReplaceAll(toolName, ",", " ")
		allowedToolNames = append(allowedToolNames, strings.Fields(toolName)...)
		return nil
	}
	flag.Func("tool", "add one or comma/space-separated list of tools to be allowed (whitelisted). may be used multiple times.", addToolWhitelist)
	flag.BoolVar(&dbflag, "db", dbflag, "this program explicity requires this db flag before using sqlite database if $ORCHARD_DB env is set")
	flag.BoolFunc("no-bash", "remove bash tool, only use toolboxes, alternatively select whitelist with -tool foo -tool bar -tool foo,bar", func(s string) error {
		orchard.AllTools = nil
		return nil
	})
	// TODO: better control for output
	flag.BoolFunc("qqq", "disable slog, hide thinking/toolcalls/toolouts, only printing content to stdout", func(s string) error {
		mllm.HideThink = true
		mllm.HideToolCalls = true
		mllm.ToolWriterOut = io.Discard
		sloglvl.Set(slog.LevelWarn)
		def := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level:     slog.LevelWarn,
			AddSource: true,
		}))
		mllm.Slog = func() *slog.Logger {
			return def
		}
		orchard.Slog = mllm.Slog
		return nil
	})
	flag.BoolFunc("qq", "hide thinking/toolcalls/toolouts, only printing content to stdout, see also -qqq", func(s string) error {
		mllm.HideThink = true
		mllm.HideToolCalls = true
		sloglvl.Set(slog.LevelWarn)
		mllm.ToolWriterOut = io.Discard
		return nil
	})
	flag.Parse()
	orchard.Slog = mllm.Slog // get -q -v flags
	if flag.NArg() != 0 {

		flag.Usage()
		os.Exit(1)
	}

	if os.Getenv("TERM") != "" && os.Getenv("NO_COLOR") == "" {
		mllm.DefaultThinkPre = colors.Dim + mllm.DefaultThinkPre
		mllm.DefaultThinkPost += colors.Reset
	}
	mllm.DefaultWriter = os.Stdout
	mllm.TruncateLogToolOutChars = 500
	if err := godotenv.Load(os.ExpandEnv("${XDG_CONFIG_HOME}/orchard/env")); err != nil {
		println("error loading env", err.Error())
		///} else {
		//	println("loaded env")
	}
	client = orchard.New()
	client.Toolbox = orchard.AllTools
	if prefix != "" {
		orchard.Custom.PromptPrefix = prefix
	}
	if suffix != "" {
		orchard.Custom.PromptSuffix = suffix + "\n" + orchard.Custom.PromptSuffix
	}

	if dir := os.Getenv("ORCHARD_TOOLS"); dir != "" {
		if strings.HasPrefix(dir, "~/") {
			dir = strings.Replace(dir, "~/", os.ExpandEnv("$HOME")+"/", 1) // TODO: more tilde + expand env elsewhere
		}
		if _, err := os.Stat(filepath.Join(dir, "toolboxes")); err == nil {
			dir = filepath.Join(dir, "toolboxes")
		}
		box, err := toolboxes.NewToolboxFS(dir)
		if err != nil {
			slog.Error("toolbox load", "err", err)
			os.Exit(1)
		}
		client.Toolbox = box.AllTools()
		client.Toolbox = append(client.Toolbox, orchard.AllTools...) // always bash tool here
		//	for _, v := range client.Toolbox {
		//		println("TOOL AVAILABLE:", v.GetName())
		//	}
	}
	if len(allowedToolNames) != 0 {
		client.Toolbox = slices.DeleteFunc(client.Toolbox, func(v mllm.Tool) bool {
			return !slices.Contains(allowedToolNames, v.GetName())
		})
		if slices.Contains(allowedToolNames, "none") {
			client.Toolbox = nil
			orchard.AllTools = nil
			client.E.Tools = nil
		}
	}
	if err := main2(client, dbflag, sleepDur); err != nil {
		fmt.Println("Error:", err)
		log.Fatalf("fatal: %+v", err)
	}
}

var Errorf = stackerr.Errorf

func isLocal(s string) bool {
	if net.ParseIP(s).String() == s {
		return true
	}
	if strings.Contains(s, ".home.arpa") {
		return true
	}
	if strings.Contains(s, "localhost") {
		return true
	}
	return false
}

var colors = struct {
	Reset string
	Dim   string
}{
	Reset: "\033[0m",
	Dim:   "\033[2m",
	/*Red:         "\033[31m",
	Green:       "\033[32m",
	Yellow:      "\033[33m",
	Blue:        "\033[34m",
	Magenta:     "\033[35m",
	Cyan:        "\033[36m",
	White:       "\033[37m",
	BlueBold:    "\033[34;1m",
	MagentaBold: "\033[35;1m",
	RedBold:     "\033[31;1m",
	YellowBold:  "\033[33;1m",*/
}

func main2(client orchard.Config, dbflag bool, sleepDur time.Duration) error {
	mllm.DefaultKeepAlive = -1
	if !isLocal(client.E.Host.Host) {
		mllm.DefaultKeepAlive = 0
	}
	wd, _ := os.Getwd()
	if mllm.DefaultThread == uuid.Nil() {
		mllm.DefaultThread = mllm.ThreadHasher(wd)
	}
	client.PreTextHookEditP1 = compactor.V1
	client.NonTextHook = func(m mllm.Message) error {
		if n := mllm.TruncateLogToolOutChars; n != 0 {
			s := m.Content
			l := len(s)
			if n > 3 && l > n {
				s = s[:n-3] + "..."
			}
			// ugly, but , does not break 2> >(jq -c)
			slog.Info("tool output", "name", m.ToolName, "content", s)
		}
		if sleepDur != 0 {
			time.Sleep(sleepDur)
		}
		return nil
	}
	if dbfile := os.Getenv("ORCHARD_DB"); dbfile != "" && dbflag {
		if strings.HasPrefix(dbfile, "~/") {
			dbfile = strings.Replace(dbfile, "~/", os.ExpandEnv("$HOME")+"/", 1)
		}
		client.DB = opendb(dbfile)
		defer closedb()
	}
	ctx := context.Background()
	//threadID := uuid.MustParse("7e0330fc-bb4a-48d7-aae5-bd666db0734a")
	client.E.Writer = os.Stdout

	if client.E.ThreadID == uuid.Nil() {
		client.E.ThreadID = mllm.DefaultThread
	}
	client.E.StreamWriter.Writer = os.Stdout

	convo, err := readMessageStdin(ctx, client.E.ThreadID)
	if err != nil {
		return err
	}
	err = client.Run(ctx, convo)
	if err != nil {
		return err
	}
	return nil
}

// using db maybe, get single message from stdin.
//
// attaches image if found
func readMessageStdin(ctx context.Context, threadID uuid.UUID) (mllm.ChatMessages, error) {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, err
	}
	x := mllm.ChatMessage{
		Role:      "user",
		Content:   string(input),
		MessageID: uuid.New(),
		CreatedAt: time.Now(),
		Thread:    threadID,
	}

	if client.DB != nil {
		// get previous
		var a mllm.ChatMessages
		a, err = client.GormThreadReader(ctx, threadID)
		if err != nil {
			return nil, err
		}
		if len(a) != 0 {
			slog.Info("thread from db", "len", len(a), "roles_5", Roles(a.TopN(5)))
		}
		// add this one
		if err := client.DB.Create(ctx, &x); err != nil {
			return nil, err
		}
		return append(a, x), nil
	}
	//	slog.Debug("single message", "prompt", x.Content)
	if err := visions.AttachImages(x.Content, &x.Message); err != nil {
		slog.Warn("could not attach image", "err", err)
	}
	return mllm.ChatMessages{x}, nil
}

func CloseDB(db *gorm.DB) {
	s, err := db.DB()
	if s != nil {
		err = s.Close()
	}
	if err != nil {
		slog.Error("cant close db", "err", err)
	} else {
		slog.Info("closed db nicely")
	}
}

func Roles(cm mllm.ChatMessages) []string {
	return cm.Roles()
}
