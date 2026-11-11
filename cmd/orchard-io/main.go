package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
	"uuid"

	"github.com/aerth/mostly/godotenv"
	"github.com/aerth/mostly/mllm"
	"github.com/aerth/mostly/stackerr"
	"github.com/aerth/orchard"
	"github.com/aerth/orchard/pkg/toolboxes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var client orchard.Config

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: false,
		Level:     slog.LevelDebug,
	})))
	orchard.Slog = mllm.Slog // get -q -v flags
	prefix := ""
	dbflag := false
	mllm.NoLogFlags = !mllm.NoLogFlags // TODO: fix mllm
	mllm.Flags()
	orchard.Flags()
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
		client.SystemPrompt = func(x *orchard.PromptInput) (string, error) {
			if x == nil {
				panic("oops")
			}
			s, err := orchard.DefaultSystemPrompt(x)
			s += "\n" + prefix
			return s, err
		}
	}
	if err := main2(client, dbflag); err != nil {
		log.Fatalln("fatal:", err)
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
	if dbfile := os.Getenv("ORCHARD_DB"); dbfile != "" && dbflag {
		if strings.HasPrefix(dbfile, "~/") {
			dbfile = strings.Replace(dbfile, "~/", os.ExpandEnv("$HOME")+"/", 1)
		}
		mllm.DefaultThread = mllm.ThreadHasher(wd) // TODO: config for filepath.Base option
		slog.Info("opening db", "thread", mllm.DefaultThread)
		gormOptions := &gorm.Config{
			TranslateError: true,
			Logger: logger.NewSlogLogger(slog.Default(), logger.Config{
				SlowThreshold:             time.Millisecond * 700,
				Colorful:                  true,
				LogLevel:                  logger.Warn,
				IgnoreRecordNotFoundError: true,
			}),
		}
		db, err := gorm.Open(sqlite.Open(dbfile), gormOptions)
		if err != nil {
			return fmt.Errorf("open sqlite %q: %w", dbfile, err)
		}
		client.DB = gorm.G[mllm.ChatMessage](db)
		defer CloseDB(db)
	}
	ctx := context.Background()
	threadID := uuid.MustParse("7e0330fc-bb4a-48d7-aae5-bd666db0734a")
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
		var a mllm.ChatMessages
		if client.DB != nil {
			// get previous
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
		}
		a = append(a, x)
		return a, nil
	}
	_, err := orchard.Create(ctx, threadID, client)
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
	roles := make([]string, len(cm))
	for i := range cm {
		roles[i] = cm[i].Role // might be empty if cm[i].Aux, caller can remove
	}
	return roles
}
