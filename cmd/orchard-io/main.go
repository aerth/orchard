package main

import (
	"context"
	"flag"
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
	orchard.Slog = mllm.Slog // get -q -v flags
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
	flag.Func("tools", "add one or comma/space-separated list of tools to be allowed (whitelisted). may be used multiple times.", addToolWhitelist)
	flag.Func("tool", "add one or comma/space-separated list of tools to be allowed (whitelisted). may be used multiple times.", addToolWhitelist)
	flag.BoolVar(&dbflag, "db", dbflag, "this program explicity requires this db flag before using sqlite database if $ORCHARD_DB env is set")
	flag.BoolFunc("no-bash", "remove bash tool, only use toolboxes", func(s string) error {
		orchard.AllTools = nil
		return nil
	})
	flag.Parse()
	if flag.NArg() != 0 {

		flag.Usage()
		os.Exit(1)
	}
	mllm.DefaultWriter = os.Stdout
	_ = godotenv.Load(os.ExpandEnv("${XDG_CONFIG_HOME}/orchard/env"))
	client.E = mllm.Default()
	client.Toolbox = orchard.AllTools
	if prefix != "" {
		orchard.Custom.PromptPrefix = prefix
	}
	if suffix != "" {
		orchard.Custom.PromptSuffix = suffix + "\n" + orchard.Custom.PromptSuffix
	}

	if dir := os.Getenv("ORCHARD_TOOLS"); dir != "" {
		if strings.HasPrefix(dir, "~/") {
			dir = strings.Replace(dir, "~/", os.ExpandEnv("$HOME")+"/", 1)
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
	}
	if err := main2(client, dbflag, sleepDur); err != nil {
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

func main2(client orchard.Config, dbflag bool, sleepDur time.Duration) error {
	mllm.DefaultKeepAlive = -1
	if !isLocal(client.E.Host.Host) {
		mllm.DefaultKeepAlive = 0
	}
	wd, _ := os.Getwd()
	if mllm.DefaultThread == uuid.Nil() {
		mllm.DefaultThread = mllm.ThreadHasher(wd) // TODO: config for filepath.Base option
	}
	client.PreTextHookEditP1 = compactor.V1
	client.NonTextHook = func(m mllm.Message) error {
		if !mllm.HideTools {
			// ugly, but , does not break 2> >(jq -c)
			slog.Info("tool output", "name", m.ToolName, "content", m.Content)
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

	// one round
	client.ThreadReader = func(ctx context.Context, threadID uuid.UUID) (mllm.ChatMessages, error) {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}
		x := mllm.ChatMessage{
			Role:    "user",
			Content: string(input),
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
			slog.Info("got thread")
			return append(a, x), nil
		}
		slog.Debug("single message", "prompt", x.Content)

		if err := visions.AttachImages(x.Content, &x.Message); err != nil {
			slog.Warn("could not attach image", "err", err)
		}
		return mllm.ChatMessages{x}, nil
	}

	// run [orchard.Create] once
	if client.E.ThreadID == uuid.Nil() {
		client.E.ThreadID = mllm.DefaultThread
	}
	err := orchard.Create(ctx, client)
	if err != nil {
		return err
	}
	return nil
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
