package main

import (
	"context"
	"flag"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"uuid"

	"github.com/aerth/mostly/godotenv"
	"github.com/aerth/mostly/mllm"
	"github.com/aerth/mostly/stackerr"
	"github.com/aerth/orchard"
	"github.com/aerth/orchard/pkg/toolboxes"
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
	mllm.NoLogFlags = !mllm.NoLogFlags // TODO: fix mllm
	mllm.Flags()
	orchard.Flags(&sloglvl)
	flag.StringVar(&prefix, "system", prefix, "for example, orchard-io -system 'explain this file, plaintext paragraph, no markdown' < README.md")
	flag.BoolVar(&dbflag, "db", dbflag, "this program explicity requires this db flag before using sqlite database if $ORCHARD_DB env is set")
	flag.BoolFunc("no-bash", "remove bash tool, only use toolboxes", func(s string) error {
		orchard.AllTools = nil
		return nil
	})
	flag.Parse()
	mllm.DefaultWriter = os.Stdout
	_ = godotenv.Load(os.ExpandEnv("${XDG_CONFIG_HOME}/orchard/env"))
	client.E = mllm.Default()
	client.Toolbox = orchard.AllTools
	if prefix != "" {
		orchard.Custom.PromptPrefix = prefix
	}
	if suffix != "" {
		orchard.Custom.PromptSuffix = prefix
	}
	if err := main2(client, dbflag); err != nil {
		log.Fatalf("fatal: %+v", err)
	}
}

var Errorf = stackerr.Errorf

func main2(client orchard.Config, dbflag bool) error {
	if dir := os.Getenv("ORCHARD_TOOLS"); dir != "" {
		if strings.HasPrefix(dir, "~/") {
			dir = strings.Replace(dir, "~/", os.ExpandEnv("$HOME")+"/", 1)
		}
		if _, err := os.Stat(filepath.Join(dir, "toolboxes")); err == nil {
			dir = filepath.Join(dir, "toolboxes")
		}
		box, err := toolboxes.NewToolboxFS(dir)
		if err != nil {
			return Errorf("%w", err)
		}
		client.Toolbox = box.AllTools()
		client.Toolbox = append(client.Toolbox, orchard.AllTools...) // always bash tool here
		//	for _, v := range client.Toolbox {
		//		println("TOOL AVAILABLE:", v.GetName())
		//	}
	}
	wd, _ := os.Getwd()
	if mllm.DefaultThread == uuid.Nil() {
		mllm.DefaultThread = mllm.ThreadHasher(wd) // TODO: config for filepath.Base option
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
	client.Writer = os.Stdout

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
		return mllm.ChatMessages{x}, nil
	}

	// run [orchard.Create] once
	if client.ThreadID == uuid.Nil() {
		client.ThreadID = mllm.DefaultThread
	}
	_, err := orchard.Create(ctx, client)
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
