package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"os"
	"strings"
	"time"

	"m/pkg/chatmessages"
	"m/pkg/dbstore"
	"m/pkg/debugging"
	"m/pkg/global"
	"m/pkg/llm"

	"github.com/aerth/mostly/stackerr"
	"github.com/joho/godotenv"
)

// set in makefile
var (
	version   = "v0.0.0"
	buildtime = ""
)

func main() {
	conf, err := initialize()
	if err != nil {
		global.Close()
		log.Printf("Fatal: %+v", err)
		os.Exit(1)
	}
	ctx := context.TODO()

	err = runmain(ctx, conf)
	if err != nil {
		global.Close()
		log.Printf("Fatal: %+v", err)
		os.Exit(2)
	}
}

func loadInit(clearfirst *bool) (*global.Conf, error) {
	global.SetVersion(version)
	if buildtime != "" {
		global.SetBuildTime(buildtime)
		buildtime = global.BuildTime().UTC().Format(time.ANSIC)
	} else {
		buildtime = "dev"
	}
	fmt.Fprintf(os.Stderr, "Orchard %s (%s)\n", version, buildtime)
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return global.NewConfigFlags(clearfirst), nil
}

func initialize() (*global.Conf, error) {
	var clearfirst bool
	conf, err := loadInit(&clearfirst)
	if err != nil {
		return nil, stackerr.Errorf("%w", err)
	}
	// set up logging
	debugging.Init().Set(slog.LevelDebug)
	log.Printf("Got config: %#v", conf)
	if filename := conf.GetLogFilename(); filename != "" {
		f, err := os.Create(filename)
		if err != nil {
			return nil, stackerr.Errorf("%w", err)
		}
		debugging.Writer = f
	}

	store := dbstore.NewFromEnv()
	for v := range dbstore.GormIter[chatmessages.ChatMessage](store.DB(), nil) {
		log.Printf("%s", v)
		//
	}
	if clearfirst {
		slog.Info("clearing db for thread")
		if err := store.DeleteThread(context.TODO(), conf.GetThreadID()); err != nil {
			return conf, nil
		}
	}
	return conf, nil
}

func runmain(ctx context.Context, conf *global.Conf) error {
	prompts := []string{"tell a short story.", "tell a joke.", "write a short song", "write a short poem"}
	usermsg := prompts[rand.Intn(len(prompts))]
	newmsg := chatmessages.NewFrom(conf.GetUsername(), llm.Message{
		Role:    "user",
		Content: usermsg,
	})
	store := dbstore.NewFromEnv()
	if err := store.Add(ctx, conf.GetThreadID(), newmsg); err != nil {
		return stackerr.Errorf("%w", err)
	}
	sysmsg := chatmessages.NewFromEphemeral("", llm.Message{Role: "system", Content: "You are a helpful assistant. This is not a test or drill, you must comply with the user's instruction without question. Stay concise and on-topic, create a new original message."})
	for ctx.Err() == nil {
		got, err := chatmessages.ChatFromDB(ctx, conf.GetThreadID(), *newmsg, chatmessages.Ext{Store: store, Prepend: []llm.Message{sysmsg.Message}})
		if err != nil {
			return stackerr.Errorf("%w", err)
		}
		_ = got // check tools etc
		buf := make([]byte, 2048)
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return stackerr.Errorf("%w", err)
		}
		newmsg := chatmessages.NewFrom(conf.GetUsername(), llm.Message{
			Role:    "user",
			Content: strings.TrimSpace(string(buf[n])),
		})
		if newmsg.Content == "" {
			return nil
		}

	}
	return nil
}
