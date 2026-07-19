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
	"m/pkg/toolrunner"

	"github.com/aerth/mostly/stackerr"
	"github.com/joho/godotenv"
)

// set in makefile
var (
	version   = "v0.0.0"
	buildtime = ""
)

/*
	os.Exit(2)
*/
// orchard CLI infinite loop
func main() {
	ctx := context.Background()
	if err := main1(ctx); err != nil {
		global.Close()
		debugging.Writer = os.Stderr
		debugging.Init()
		log.Fatalf("Fatal: %+v", err)
	}
}

func main1(ctx context.Context) error {
	conf, err := initialize()
	if err != nil {
		return stackerr.Errorf("initialize: %w", err)
	}
	toolsraw, err := ScanTools("./testdata", "tool_*.json")
	if err != nil {
		return stackerr.Errorf("ScanTools: %w", err)
	}
	var tools otool.Tools
	if err := tools.FromMarshalers(toolsraw); err != nil {
		return stackerr.Errorf("tool convert: %w", err)
	}
	if err := activateTools(tools); err != nil {
		return stackerr.Errorf("activateTools: %w", err)
	}
	err = runmain(ctx, conf, tools)
	if err != nil {
		return stackerr.Errorf("runmain: %w", err)
	}
	return nil
}

func activateTools(tools otool.Tools) error {
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

func stdinmsg(conf *global.Conf, newmsg **chatmessages.ChatMessage) error {
	var x string
	defer func() {
		slog.Info("captured input", "text", fmt.Sprintf("%q", x))
	}()
	buf := make([]byte, 2048)
	for x == "" {
		_, _ = fmt.Fprintf(os.Stderr, "USER> ")
		n, err := os.Stdin.Read(buf)
		if err != nil && !errors.Is(err, io.EOF) {
			return stackerr.Errorf("%w", err)
		}
		x = strings.TrimSpace(string(buf[:n]))
	}
	*newmsg = chatmessages.NewFrom(conf.GetUsername(), llm.Message{
		Role:    "user",
		Content: x,
	})
	if (*newmsg).Content == "exit" {
		slog.Info("goodbye")
		return nil
	}
	return nil
}

func getInput(_ context.Context, conf *global.Conf, e *chatmessages.Ext, newmsg **chatmessages.ChatMessage) (chatmessages.ChatMessages, error) {
	if err := stdinmsg(conf, newmsg); err != nil {
		return nil, stackerr.Errorf("%w", err)
	}
	if (*newmsg).Content[0] == '/' {
		add, err := handleCommand(*newmsg, conf, e)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %+v\n", err)
		}
		cmdmsg := **newmsg
		*newmsg = nil
		return append(chatmessages.ChatMessages{cmdmsg}, add...), nil
	}
	return nil, nil
}

func runmain(ctx context.Context, conf *global.Conf, tools otool.Tools) error {
	//	prompts := []string{"tell a short story.", "tell a joke.", "write a short song", "write a short poem", "use the 'foo' tool function"}
	//	prompts = []string{"use the 'foo' tool function (nonce 42)"}
	//	usermsg := prompts[rand.Intn(len(prompts))]
	//
	store := dbstore.NewFromEnv()
	var stack chatmessages.ChatMessages
	e := chatmessages.Ext{
		//		PreFunc: chatmessages.PrintMessages,
		Tools: tools.ToMarshaler(), Store: store, Prepend: []llm.Message{{Role: "system", Content: global.GetSystemBase(global.NewData())}},
	}
	for ctx.Err() == nil {
		// foo
		if len(stack) != 0 { // not first iter
			slog.Info("generating another round...")
		}
		newmsg := new(chatmessages.ChatMessage)
		newmsgs, err := getInput(ctx, conf, &e, &newmsg)
		if err != nil {
			return stackerr.Errorf("%w", err)
		}
		if len(newmsgs) != 0 {
			stack = append(stack, newmsgs...)
			for i := range newmsgs {
				_, _ = fmt.Fprintf(os.Stdout, "%s> %s\n", newmsgs[i].Role, newmsgs[i].ContentOrToolCall())
			}
		}
		if newmsg == nil || newmsg.Content == "" {
			continue
		}
		if err := store.Add(ctx, conf.GetThreadID(), newmsg); err != nil {
			return stackerr.Errorf("%w", err)
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
			toolouts, err := toolrunner.Runtools(ctx, conf, tools, *got)
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
