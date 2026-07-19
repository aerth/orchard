package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	"m/pkg/chatmessages"
	"m/pkg/dbstore"
	"m/pkg/debugging"
	"m/pkg/global"
	"m/pkg/llm"
	"m/pkg/otool"

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
	toolsraw, err := ScanTools("./testdata", "tool_*.json")
	if err != nil {
		global.Close()
		log.Printf("Fatal: %+v", err)
		os.Exit(1)
	}
	//json.NewEncoder(os.Stderr).Encode(tools)
	var tools otool.Tools
	if err := tools.FromMarshalers(toolsraw); err != nil {
		global.Close()
		log.Printf("Fatal: %+v", err)
		os.Exit(1)

	}
	activateTools(tools)
	err = runmain(ctx, conf, tools)
	if err != nil {
		global.Close()
		debugging.Writer = os.Stderr
		debugging.Init()
		log.Printf("Fatal: %+v", err)
		os.Exit(2)
	}
}

func activateTools(tools otool.Tools) error {
	for i := range tools {
		if tools[i].Name == "foo" {
			tools[i].SetExec(
				func(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
					var call struct {
						ID        string `json:"id"`
						Arguments struct {
							Nonce uint `json:"some_nonce"`
						} `json:"arguments"`
					}
					if err := json.Unmarshal(caller.ToolCalls[i], &call); err != nil {
						return nil, err
					}
					return chatmessages.ChatMessages{*chatmessages.NewFrom("", llm.Message{Role: "tool", ToolName: "foo", ToolCallID: call.ID, Content: fmt.Sprintf("foo has completed (the nonce this tool received was '%d')", call.Arguments.Nonce)})}, nil
				},
			)
		} else {
			return fmt.Errorf("not implemented")
		}
	}
	return nil
}

func stdinmsg(conf *global.Conf, newmsg **chatmessages.ChatMessage) error {
	var x string
	for x == "" {
		_, _ = fmt.Fprintf(os.Stderr, "USER> ")
		buf := make([]byte, 2048)
		n, err := os.Stdin.Read(buf)
		if err != nil && !errors.Is(err, io.EOF) {
			return stackerr.Errorf("%w", err)
		}
		x = strings.TrimSpace(string(buf[:n]))
		*newmsg = chatmessages.NewFrom(conf.GetUsername(), llm.Message{
			Role:    "user",
			Content: x,
		})
	}
	if (*newmsg).Content == "exit" {
		slog.Info("goodbye")
		return nil
	}
	return nil
}

func runmain(ctx context.Context, conf *global.Conf, tools otool.Tools) error {
	//	prompts := []string{"tell a short story.", "tell a joke.", "write a short song", "write a short poem", "use the 'foo' tool function"}
	//	prompts = []string{"use the 'foo' tool function (nonce 42)"}
	//	usermsg := prompts[rand.Intn(len(prompts))]
	//
	store := dbstore.NewFromEnv()
	var stack chatmessages.ChatMessages
	e := chatmessages.Ext{Tools: tools.ToMarshaler(), Store: store, Prepend: []llm.Message{{Role: "system", Content: global.GetSystemBase(global.NewData())}}}
	for ctx.Err() == nil {
		var newmsg *chatmessages.ChatMessage
		if err := stdinmsg(conf, &newmsg); err != nil {
			return stackerr.Errorf("%w", err)
		}
		if newmsg.Content[0] == '/' {
			add, err := handleCommand(newmsg, conf, &e)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %+v\n", err)
			}
			for i := range add { // if /system , for example, there are some new ephemeral messages to add to this round
				if err := store.Add(ctx, conf.GetThreadID(), &add[i]); err != nil {
					return stackerr.Errorf("%w", err)
				}
			}
			continue
		}
		if err := store.Add(ctx, conf.GetThreadID(), newmsg); err != nil {
			return stackerr.Errorf("%w", err)
		}
		if newmsg.Content == "" {
			if err := stdinmsg(conf, &newmsg); err != nil {
				return stackerr.Errorf("%w", err)
			}
		}
		stack = append(stack, *newmsg)
		//	got, err := chatmessages.ChatFromDB(ctx, conf.GetThreadID(), nil, chatmessages.Ext{Tools: tools.ToMarshaler(), Store: store, Prepend: []llm.Message{sysmsg}})
		got, err := chatmessages.Chat(ctx, conf.GetThreadID(), stack, e)
		if err != nil {
			return stackerr.Errorf("%w", err)
		}
		stack = append(stack, *got)
		_, _ = fmt.Fprintf(os.Stdout, "BOT> %s\n", got.ContentOrToolCall())

		if len(got.ToolCalls) != 0 {
			toolouts, err := runtools(ctx, conf, tools, got.Message)
			if WrapToolErrors && err != nil {
				toolouts = append(toolouts, WrapToolError(err)...)
				err = nil
			}
			if err != nil {
				return stackerr.Errorf("running tools: %w", err)
			}
			for i := range toolouts {
				_, _ = fmt.Fprintf(os.Stdout, "TOOL> %s\n", toolouts[i].ContentOrToolCall())
			}
			if err := store.Add(ctx, conf.GetThreadID(), chatmessages.Ptrs(toolouts)...); err != nil {
				return stackerr.Errorf("adding tool output to db: %w", err)
			}
			stack = append(stack, toolouts...)
			continue
		}

	}
	return nil
}

var WrapToolErrors = true

func WrapToolError(err error) chatmessages.ChatMessages {
	return chatmessages.ChatMessages{*chatmessages.NewChatMessage("tool", fmt.Sprintf("Error running tool: %+v", err))}
}

func runtools(ctx context.Context, conf *global.Conf, tools otool.Tools, caller llm.Message) (chatmessages.ChatMessages, error) {
	var toolouts chatmessages.ChatMessages
	toolCalls := caller.ToolCalls
	for toolCallIndex := range toolCalls {
		var call struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		}
		if err := json.Unmarshal(toolCalls[toolCallIndex], &call); err != nil {
			return toolouts, stackerr.Errorf("%w", err)
		}
		var tool otool.Tool
		for _, t := range tools {
			if t.Name == call.Function.Name {
				tool = t
				break
			}
		}
		if tool.Name != call.Function.Name {
			slog.Warn("Not Running tools...", "i", toolCallIndex, "call", call)
			return toolouts, stackerr.Errorf("cant find tool named %s", call.Function.Name)
		}
		slog.Info("Running tools...", "i", toolCallIndex, "call", call)
		x, err := tool.Exec(ctx, caller, toolCallIndex)
		toolouts = append(toolouts, x...)
		if err != nil {
			return toolouts, err
		}

	}
	return toolouts, nil
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
		log.Printf("%s: %#v", v, v)
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
