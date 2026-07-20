package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"m/cmdpkg"
	"m/cmdpkg/datadata"
	"m/pkg/builtins"
	"m/pkg/builtins/implv1"
	"m/pkg/chat"
	"m/pkg/chatmessages"
	"m/pkg/dbstore"
	"m/pkg/debugging"
	"m/pkg/global"
	"m/pkg/llm"
	"m/pkg/otool"
	"m/pkg/toolrunner"

	"github.com/aerth/mostly/stackerr"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// set in makefile
var (
	version   = "v0.0.0"
	buildtime = ""
)

func Sig() {
	//ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM)
	ch := make(chan os.Signal, 1000)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM)
	for sig := range ch {
		slog.Info("received signal", "sig", sig)
		global.Cancel(stackerr.Errorf("received signal %s", sig))
	}
}

var builtinTools otool.Tools

// orchard CLI infinite loop, simple stdin stdout, minimal stderr.
// debug logs to log file or null
func main() {
	global.SetVersion(version, "stdio")
	var err error
	builtinTools, err = builtins.LoadTools(map[string]otool.ExecFunc{
		"read_file": implv1.ReadFile,
		//	"write_file":       noop,
		//	"run_go_command":   noop,
		"list_directory": implv1.ListDirectory,
		//	"get_package_info": noop,
	})
	if err != nil {
		log.Fatalf("could not load builtin tools: %+v", err)
	}
	if len(builtinTools) == 0 {
		slog.Error("could not")
		log.Fatalf("o")
	}
	if buildtime != "" {
		global.SetBuildTime(buildtime)
		buildtime = global.BuildTime().UTC().Format(time.ANSIC)
	} else {
		buildtime = "dev"
	}
	fmt.Fprintf(os.Stderr, "Press ctrl+d to end each message, press ctrl+c to cancel generation.\n")
	ctx := context.Background()
	go Sig()
	if err := main1(ctx); err != nil {
		global.Close()
		debugging.Writer = os.Stderr
		debugging.Init()
		log.Fatalf("Fatal: %+v", err)
	}
}

func main1(ctx context.Context) error {
	conf, err := cmdpkg.Initialize()
	if err != nil {
		return stackerr.Errorf("initialize: %w", err)
	}
	var tools otool.Tools
	/*
		toolsraw, err := ScanTools("./testdata", "tool_*.json")
		if err != nil {
			return stackerr.Errorf("ScanTools: %w", err)
		}
		if err := tools.FromMarshalers(toolsraw); err != nil {
			return stackerr.Errorf("tool convert: %w", err)
		}
		if err := cmdpkg.ActivateTools(tools); err != nil {
			return stackerr.Errorf("activateTools: %w", err)
		}*/
	tools = append(tools, builtinTools...)
	err = runmain(ctx, conf, tools)
	if err != nil {
		return stackerr.Errorf("runmain: %w", err)
	}
	return nil
}

func runmain(ctx context.Context, conf *global.Conf, tools otool.Tools) error {
	//	prompts := []string{"tell a short story.", "tell a joke.", "write a short song", "write a short poem", "use the 'foo' tool function"}
	//	prompts = []string{"use the 'foo' tool function (nonce 42)"}
	//	usermsg := prompts[rand.Intn(len(prompts))]
	//
	store := dbstore.NewFromEnv()
	e := chat.Ext{
		//	PreFunc: chat.PrintMessages,
		Tools: tools.ToMarshaler(), Store: store,
	}

	stack, err := store.LoadThread(ctx, conf.GetThreadID(), 4)
	if err != nil {
		return err
	}
	startsize := len(stack)
	empties := 0
	for ctx.Err() == nil {
		if len(stack)-startsize != 0 && stack.Top().Role == "tool" {
			slog.Info("generating another round...")
		} else {
			newmsg := new(chatmessages.ChatMessage)
			newmsg.From = conf.GetUsername()
			sctx := global.NewCancel(ctx)
			_, err := getInput(sctx, conf, &e, &newmsg)
			if err != nil {
				return stackerr.Errorf("%w", err)
			}
			{
				if newmsg != nil && cmdpkg.IsCommand(newmsg.Content) {
					add, err := cmdpkg.HandleCommand(newmsg, conf, &e, &stack)
					// dont have caller generate
					cmdmsg := *newmsg
					newmsg = nil
					if cmdmsg.Content != "/clear" {
						stack = append(stack, cmdmsg)
					}
					if err != nil {
						return err
					}
					for _, v := range add {
						_, _ = fmt.Fprintf(os.Stdout, "%s> %s\n", v.Role, v.ContentOrToolCall())
					}
					stack = append(stack, add...)
					continue
				}
			}
			if newmsg == nil || newmsg.Content == "" {
				empties++
				if empties > 4 {
					break
				}
				continue
			}
			empties = 0
			if err := store.Add(ctx, conf.GetThreadID(), newmsg); err != nil {
				return stackerr.Errorf("%w", err)
			}
			stack = append(stack, *newmsg)
		}
		//	got, err := chatmessages.ChatFromDB(ctx, conf.GetThreadID(), nil, chatmessages.Ext{Tools: tools.ToMarshaler(), Store: store, Prepend: []llm.Message{sysmsg}})

		e.Prepend = []llm.Message{{Role: "system", Content: global.GetSystemBase(datadata.NewData(stack))}}
		sctx := global.NewCancel(ctx)
		e.Tools = tools.ToMarshaler()
		got, err := chat.Chat(sctx, conf.GetThreadID(), stack, e)
		if err != nil {
			return stackerr.Errorf("%w", err)
		}
		stack = append(stack, *got)
		_, _ = fmt.Fprintf(os.Stdout, "BOT> %s\n", got.ContentOrToolCall())

		if len(got.ToolCalls) != 0 {
			toolouts, err := toolrunner.Runtools(ctx, conf, tools, *got)
			if cmdpkg.WrapToolErrors && err != nil {
				toolouts = append(toolouts, cmdpkg.WrapToolError(err)...)
				err = nil
			}
			if err != nil {
				return stackerr.Errorf("running tools: %w", err)
			}
			if len(toolouts) == 0 {
				toolouts = append(toolouts, cmdpkg.WrapToolError(fmt.Errorf("no output"))...)
			}
			for i := range toolouts {
				_, _ = fmt.Fprintf(os.Stdout, "TOOL> %s\n", toolouts[i].ContentOrToolCall())
			}
			if err := store.Add(ctx, conf.GetThreadID(), chat.Ptrs(toolouts)...); err != nil {
				return stackerr.Errorf("adding tool output to db: %w", err)
			}
			stack = append(stack, toolouts...)
			continue
		}

	}
	return nil
}

var titlecaser = cases.Title(language.AmericanEnglish)

func ReadMessage(r io.Reader, from, role string, newmsg **chatmessages.ChatMessage) error {
	var x string
	var err error
	defer func() { slog.Info("captured input", "text", x, "err", err) }()
	// prompt
	_, _ = fmt.Fprintf(os.Stderr, "\n%s: ", titlecaser.String(from)) // eg: Chief:
	// read stdin until EOF
	var buf []byte
	buf, err = io.ReadAll(r)
	x = string(buf)
	if x != "" && errors.Is(err, io.EOF) {
		err = nil
	}
	if err != nil {
		return err
	}

	//
	if *newmsg == nil {
		*newmsg = chatmessages.New(llm.Message{})
	}
	if (*newmsg).From != from {
		(*newmsg).From = from
	}
	if (*newmsg).Role != role {
		(*newmsg).Role = role
	}

	(*newmsg).Content = strings.TrimSpace(x)
	return nil
}

func stdinmsg(ctx context.Context, conf *global.Conf, newmsg **chatmessages.ChatMessage) error {
	if true {
		return ReadMessage(os.Stdin, conf.GetUsername(), "user", newmsg)
	}
	var x string
	var err error
	defer func() { slog.Info("captured input", "text", x, "err", err) }()
	_, err = fmt.Fprintf(os.Stderr, "\nUser: ")
	if err != nil {
		return err
	}
	buf, err := io.ReadAll(os.Stdin)

	x = strings.TrimSpace(string(buf))
	if x == "" && errors.Is(err, io.EOF) {
		return err
	}
	if x == "" {
		slog.Info("nil message")
		*newmsg = nil
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	*newmsg = chatmessages.NewFrom(conf.GetUsername(), llm.Message{
		Role:    "user",
		Content: x,
	})
	if strings.TrimSpace((*newmsg).Content) == "exit" {
		slog.Info("goodbye")
		return nil
	}
	slog.Info("got message", "text", x)
	return nil
}

func getInput(ctx context.Context, conf *global.Conf, e *chat.Ext, newmsg **chatmessages.ChatMessage) (chatmessages.ChatMessages, error) {
	slog.Info("getting input")
	if err := stdinmsg(ctx, conf, newmsg); err != nil {
		return nil, stackerr.Errorf("%w", err)
	}
	return nil, ctx.Err()
}
