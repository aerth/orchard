package cmdpkg

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"m/pkg/chatmessages"
	"m/pkg/dbstore"
	"m/pkg/global"
	"m/pkg/llm"
)

func HandleCommand(newmsg *chatmessages.ChatMessage, conf *global.Conf, e *chatmessages.Ext) (chatmessages.A, error) {
	fields := strings.Fields(newmsg.Content)
	cmd := strings.TrimPrefix(fields[0], "/")
	slog.Info("commander", "cmd", cmd)
	switch cmd {
	case "help":
		println("help mode soon")
	case "debug":
		if e.PreFunc == nil {
			slog.Info("adding prefunc")
			e.PreFunc = chatmessages.PrintMessages
		} else {
			slog.Info("removing prefunc")
			e.PreFunc = nil
		}
		return nil, nil
	case "clear":
		err := e.Store.(*dbstore.PostgresDBStore).DeleteThread(context.TODO(), conf.GetThreadID())
		return nil, err
	case "model":
		return nil, os.Setenv("OLLAMA_MODEL", fields[1])
	case "think":
		return nil, os.Setenv("OLLAMA_THINK", fields[1])
	case "system":
		return chatmessages.A{*chatmessages.NewFromEphemeral("", llm.Message{Role: "system", Content: strings.Join(fields[1:], " ")})}, nil
	case "setsystem":
		e.Prepend[0].Content = strings.Join(fields[1:], " ")
		//return chatmessages.A{*chatmessages.New(llm.Message{Role: "system", Content: strings.Join(fields[1:], " ")})}, nil
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown command")
	}
	return nil, nil
}
