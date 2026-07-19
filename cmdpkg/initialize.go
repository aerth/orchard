// Package cmdpkg for shared main-package things
package cmdpkg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"

	"m/pkg/chatmessages"
	"m/pkg/dbstore"
	"m/pkg/debugging"
	"m/pkg/global"
	"m/pkg/llm"
	"m/pkg/otool"

	"github.com/aerth/mostly/stackerr"
	"github.com/joho/godotenv"
)

func ActivateTools(tools otool.Tools) error {
	for i := range tools {
		if tools[i].Name == "foo" {
			tools[i].SetExec(
				func(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
					var call struct {
						ID       string `json:"id"`
						Function struct {
							Arguments struct {
								Nonce uint `json:"some_nonce"`
							} `json:"arguments"`
						} `json:"function"`
					}
					if err := json.Unmarshal(caller.ToolCalls[i], &call); err != nil {
						return nil, err
					}
					return chatmessages.ChatMessages{*chatmessages.NewFrom("", llm.Message{Role: "tool", ToolName: "foo", ToolCallID: call.ID, Content: fmt.Sprintf("foo has completed (the nonce this tool received was '%d')", call.Function.Arguments.Nonce)})}, nil
				},
			)
		} else {
			return fmt.Errorf("not implemented")
		}
	}
	return nil
}

var WrapToolErrors = true

func WrapToolError(err error) chatmessages.ChatMessages {
	return chatmessages.ChatMessages{*chatmessages.NewChatMessage("tool", fmt.Sprintf("Error running tool: %+v", err))}
}

func loadInit(clearfirst *bool) (*global.Conf, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return global.NewConfigFlags(clearfirst), nil
}

func Initialize() (*global.Conf, error) {
	var clearfirst bool
	conf, err := loadInit(&clearfirst)
	if err != nil {
		return nil, stackerr.Errorf("%w", err)
	}
	// set up logging
	// log.Printf("Got config: %#v", conf)
	if filename := conf.GetLogFilename(); filename != "" {
		f, err := os.Create(filename)
		if err != nil {
			return nil, stackerr.Errorf("%w", err)
		}
		debugging.Writer = f
	}
	debugging.Init().Set(slog.LevelDebug)
	store := dbstore.NewFromEnv()
	for v := range dbstore.GormIter[chatmessages.ChatMessage](store.DB(), nil) {
		log.Printf("iter: %s", v)
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
