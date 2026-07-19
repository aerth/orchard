package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"m/pkg/chatmessages"
	"m/pkg/dbstore"
	"m/pkg/global"
)

func handleCommand(newmsg *chatmessages.ChatMessage, conf *global.Conf, e *chatmessages.Ext) (chatmessages.A, error) {
	fields := strings.Fields(newmsg.Content)
	switch strings.TrimPrefix(fields[0], "/") {
	case "help":
		println("help mode soon")
	case "clear":
		err := e.Store.(*dbstore.PostgresDBStore).DeleteThread(context.TODO(), conf.GetThreadID())
		return nil, err
	case "model":
		return nil, os.Setenv("OLLAMA_MODEL", fields[1])
	case "think":
		return nil, os.Setenv("OLLAMA_THINK", fields[1])
	case "system":

		e.Prepend[0].Content = strings.Join(fields[1:], " ")
		//return chatmessages.A{*chatmessages.New(llm.Message{Role: "system", Content: strings.Join(fields[1:], " ")})}, nil
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown command")
	}
	return nil, nil
}
