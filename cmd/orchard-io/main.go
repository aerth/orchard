// Command orchard-io
//
// For example,
//
// ```bash
// bin/orchard-chat -clear <<<"summarize Makefile, then summarize cmd/orchard-chat/main.go" 2>/dev/null
// ```
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
	"m/cmdpkg/rege"
	"m/cmdpkg/spinner"
	"m/cmdpkg/templatedata"
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
		_, _ = fmt.Fprintf(os.Stderr, " %d more times to quit\n", global.Remaining())
	}
}

var builtinTools otool.Tools

// orchard CLI infinite loop, simple stdin stdout, minimal stderr.
// debug logs to log file or null
func main() {
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
		panic("no builtin tools")
	}
	//
	global.SetVersion(version, "stdio")
	if buildtime != "" {
		global.SetBuildTime(buildtime)
		buildtime = global.BuildTime().UTC().Format(time.ANSIC)
	} else {
		buildtime = "dev"
	}

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
	err = runmain(ctx, conf)
	if err != nil {
		return stackerr.Errorf("runmain: %w", err)
	}
	return nil
}

func runmain(ctx context.Context, conf *global.Conf) error {
	showChar := false

	if showChar {
		fmt.Fprintf(os.Stderr, "Press ctrl+d to end each message, press ctrl+c to cancel generation.\n")
	}
	//	prompts := []string{"tell a short story.", "tell a joke.", "write a short song", "write a short poem", "use the 'foo' tool function"}
	//	prompts = []string{"use the 'foo' tool function (nonce 42)"}
	//	usermsg := prompts[rand.Intn(len(prompts))]
	store := dbstore.NewFromEnv()
	e := chat.Ext{
		//	PreFunc: chat.PrintMessages,
		// Tools: builtinTools.ToMarshaler(),
		Store: store,
	}

	stack, err := store.LoadThread(ctx, conf.GetThreadID(), 4)
	if err != nil {
		return err
	}
	newmsg := new(chatmessages.ChatMessage)
	newmsg.From = conf.GetUsername()
	if _, e := os.Stdin.Read(nil); e != nil {
		return e
	}
	if showChar {
		_, _ = fmt.Fprintf(os.Stderr, "%s: ", titlecaser.String(conf.GetUsername())) // eg: Chief:
	}
	err = ReadMessage(os.Stdin, conf.GetUsername(), "user", &newmsg)
	if err != nil {
		return stackerr.Errorf("ReadMessage: %w", err)
	}
	if newmsg == nil || newmsg.Content == "" {
		return nil
	}
	if newmsg != nil && cmdpkg.IsCommand(newmsg.Content) {
		add, err := cmdpkg.HandleCommand(newmsg, conf, &e, &stack)
		// dont have caller generate
		cmdmsg := *newmsg
		newmsg = nil
		if cmdmsg.Content != "/clear" {
			stack = append(stack, cmdmsg)
		}
		stack = append(stack, add...)
		for _, v := range add {
			_, _ = fmt.Fprintf(os.Stdout, "%s> %s\n", v.Role, v.ContentOrToolCall())
		}
		if err != nil {
			return stackerr.Errorf("HandleCommand: %w", err)
		}
		if add.TopRole("user").Content == "" {
			return nil
		}
	}
	if newmsg == nil || newmsg.Content == "" {
		return nil
	}

	if ss := rege.FindImagePaths(newmsg.Content); len(ss) != 0 {
		newmsg.Attachments = ss
	}
	if err := store.Add(ctx, conf.GetThreadID(), newmsg); err != nil {
		return stackerr.Errorf("%w", err)
	}
	stack = append(stack, *newmsg)
	cl := debugging.NewConsoleLogger(os.Stdout)
	cl.HideToolCalls = conf.IsHideToolCalls()
Chat:
	e.Prepend = []llm.Message{{Role: "system", Content: global.GetSystemBase(templatedata.NewData(stack))}}
	sctx := global.NewCancel(ctx)
	e.Tools = builtinTools.ToMarshaler()
	cl.Reset()
	got := chatmessages.NewFromRole(e.From, "assistant", "")
	slog.Info("Chatting")
	spin := spinner.Start(sctx)
	e.Handlers = []llm.ChatStreamHandlerFunc{spin.HandleChatResponse, cl.HandleChatResponse, got.GatherChatResponse}
	_, err = chat.Chat(sctx, conf.GetThreadID(), stack, e)
	_ = spin.HandleChatResponse(llm.ChatResponse{})
	if err != nil {
		return stackerr.Errorf("%w", err)
	}

	if e.Store != nil {
		if err := e.Store.Add(context.WithoutCancel(ctx), conf.GetThreadID(), got); err != nil {
			return stackerr.Errorf("%w", err)
		}
	}
	stack = append(stack, *got)

	/*
		// show output
		if showChar {
			fmt.Fprintf(os.Stderr, "\nBOT> ")
		}
		w := os.Stdout
		if len(got.ToolCalls) != 0 {
			w = os.Stderr
		}
		_, _ = fmt.Fprintf(w, "%s\n", got.ContentOrToolCall())
	*/
	// handle tools
	if len(got.ToolCalls) != 0 {
		toolouts, err := toolrunner.Runtools(ctx, conf, builtinTools, *got)
		if cmdpkg.WrapToolErrors && err != nil {
			toolouts = append(toolouts, cmdpkg.WrapToolError(err)...)
			err = nil
		}
		if len(toolouts) == 0 {
			toolouts = append(toolouts, cmdpkg.WrapToolError(fmt.Errorf("no output"))...)
		}
		if !conf.IsHideToolOuts() {
			for i := range toolouts {
				if showChar {
					_, _ = fmt.Fprintf(os.Stderr, "TOOL> ")
				}
				_, _ = fmt.Fprintf(os.Stderr, "\n```\n%s\n```\n", toolouts[i].ContentOrToolCall())
			}
		}
		if err != nil {
			return stackerr.Errorf("running tools: %w", err)
		}
		if err := store.Add(ctx, conf.GetThreadID(), chat.Ptrs(toolouts)...); err != nil {
			return stackerr.Errorf("adding tool output to db: %w", err)
		}
		stack = append(stack, toolouts...)
		goto Chat
	}

	return nil
}

var titlecaser = cases.Title(language.AmericanEnglish)

func ReadMessage(r io.Reader, from, role string, newmsg **chatmessages.ChatMessage) error {
	var x string
	var err error
	defer func() { slog.Info("captured input", "text", x, "err", err) }()
	// prompt
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
