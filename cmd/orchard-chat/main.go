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
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"m/cmdpkg"
	"m/cmdpkg/datadata"
	"m/pkg/builtins"
	"m/pkg/chat"
	"m/pkg/chatmessages"
	"m/pkg/dbstore"
	"m/pkg/debugging"
	"m/pkg/global"
	"m/pkg/llm"
	"m/pkg/otool"
	"m/pkg/toolrunner"

	"github.com/aerth/mostly/stackerr"
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

func noop(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
	return nil, nil
}

// orchard CLI infinite loop, simple stdin stdout, minimal stderr.
// debug logs to log file or null
func main() {
	global.SetVersion(version, "stdio")
	var err error
	builtinTools, err = builtins.LoadTools(map[string]otool.ExecFunc{
		"read_file":        noop,
		"write_file":       noop,
		"run_go_command":   noop,
		"list_directory":   noop,
		"get_package_info": noop,
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
	toolsraw, err := ScanTools("./testdata", "tool_*.json")
	if err != nil {
		return stackerr.Errorf("ScanTools: %w", err)
	}
	var tools otool.Tools
	if err := tools.FromMarshalers(toolsraw); err != nil {
		return stackerr.Errorf("tool convert: %w", err)
	}
	if err := cmdpkg.ActivateTools(tools); err != nil {
		return stackerr.Errorf("activateTools: %w", err)
	}
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
	var stack chatmessages.ChatMessages
	e := chat.Ext{
		//	PreFunc: chat.PrintMessages,
		Tools: tools.ToMarshaler(), Store: store,
	}
	var err error
	stack, err = store.LoadThread(ctx, conf.GetThreadID(), 4)
	if err != nil {
		return err
	}
	for ctx.Err() == nil {
		if len(stack) != 0 && stack.Top().Role == "tool" {
			slog.Info("generating another round...")
		} else {
			newmsg := new(chatmessages.ChatMessage)
			newmsg.From = conf.GetUsername()
			sctx := global.NewCancel(ctx)
			altnewmsgs, err := getInput(sctx, conf, &e, &newmsg)
			if err != nil {
				return stackerr.Errorf("%w", err)
			}
			if len(altnewmsgs) != 0 {
				stack = append(stack, altnewmsgs...)
				for _, v := range altnewmsgs {
					_, _ = fmt.Fprintf(os.Stdout, "%s> %s\n", v.Role, v.ContentOrToolCall())
				}
			}
			if newmsg == nil || newmsg.Content == "" {
				continue
			}
			if err := store.Add(ctx, conf.GetThreadID(), newmsg); err != nil {
				return stackerr.Errorf("%w", err)
			}
			stack = append(stack, *newmsg)
		}
		//	got, err := chatmessages.ChatFromDB(ctx, conf.GetThreadID(), nil, chatmessages.Ext{Tools: tools.ToMarshaler(), Store: store, Prepend: []llm.Message{sysmsg}})

		e.Prepend = []llm.Message{{Role: "system", Content: global.GetSystemBase(datadata.NewData(stack))}}
		sctx := global.NewCancel(ctx)
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

func stdinmsg(ctx context.Context, conf *global.Conf, newmsg **chatmessages.ChatMessage) error {
	var x string
	defer func() {
		slog.Info("captured input", "text", x)
	}()
	buf := make([]byte, 10000)

	for lineno := 1; ctx.Err() == nil && (strings.TrimSpace(x) == "" || strings.HasSuffix(x, "\n")); lineno++ {
		if strings.TrimSpace(x) == "" {
			_, _ = fmt.Fprintf(os.Stderr, "\nUSER> ")
		}
		n, err := os.Stdin.Read(buf)
		if err != nil && !errors.Is(err, io.EOF) {
			return stackerr.Errorf("%w", err)
		}
		if lineno == 2 && n == 1 && buf[0] == '\n' {
			break
		}
		if n == 1 && buf[0] == '\n' && strings.TrimSpace(x) == "" {
			slog.Warn("loop", "lineno", lineno, "x", x)
			continue
		}
		x += string(buf[:n])
		if err == io.EOF {
			break
		}
		if x == "crash\n" {
			debug.PrintStack()
			return stackerr.Errorf("cowabunga")
		}
		if x == "exit\n" {
			return stackerr.Errorf("exit successfully")
		}
		slog.Info("going again", "new", string(buf[:n]), "total", x)
	}
	x = strings.TrimSpace(x)
	if x == "" {
		return nil
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

func getInput(ctx context.Context, conf *global.Conf, e *chat.Ext, newmsg **chatmessages.ChatMessage) (chatmessages.ChatMessages, error) {
	if err := stdinmsg(ctx, conf, newmsg); err != nil {
		return nil, stackerr.Errorf("%w", err)
	}
	if *newmsg != nil && cmdpkg.IsCommand((*newmsg).Content) {
		add, err := cmdpkg.HandleCommand(*newmsg, conf, e)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %+v\n", err)
		}
		cmdmsg := **newmsg
		*newmsg = nil
		return append(chatmessages.ChatMessages{cmdmsg}, add...), nil
	}
	return nil, nil
}
