// Command orchard-io
//
// For example,
//
// ```bash
// orchard-io -q -clear <<<"summarize Makefile, then summarize cmd/orchard-chat/main.go" 2>/dev/null
// ```
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
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"m/cmdpkg"
	"m/cmdpkg/basher1"
	"m/cmdpkg/spinner"
	"m/pkg/builtins"
	"m/pkg/builtins/implv1"
	"m/pkg/builtins/implv2"
	"m/pkg/chat"
	"m/pkg/chatmessages"
	"m/pkg/dbstore"
	"m/pkg/debugging"
	"m/pkg/global"
	"m/pkg/llm"
	"m/pkg/loaders/toolboxes"
	"m/pkg/otool"

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
	ch := make(chan os.Signal, 1000)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM)
	for sig := range ch {
		slog.Info("received signal", "sig", sig)
		for range 6 {
			global.CancelCtrlC(stackerr.Errorf("received signal %s", sig))
		}
	}
}

var builtinTools otool.Tools

func init() {
	implv2.ConfirmAction = cmdpkg.Cheatcode
}

// orchard CLI infinite loop, simple stdin stdout, minimal stderr.
// debug logs to log file or null
func main() {
	signal.Ignore(syscall.SIGPIPE) // ignore pipe sig, let read/write error do its thing
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
		fmt.Fprintf(os.Stderr, "Fatal: %+v\n", err)
		slog.Error("fatal", "err", err)
		global.Close()
		cmdpkg.Writer = os.Stderr
		cmdpkg.InitLog()
		if errors.Is(err, context.Canceled) {
			println("(cancelled)")
		} else {
			log.Fatalf("Fatal: %+v", err)
		}
	}
}

func main1(ctx context.Context) error {
	var clearfirst bool
	conf, err := cmdpkg.Initialize(&clearfirst)
	if err != nil {
		return stackerr.Errorf("initialize: %w", err)
	}
	sql, err := NewPostgres(&chatmessages.ChatMessage{})
	if err != nil {
		return stackerr.Errorf("db: %w", err)
	}
	store := dbstore.New(sql)
	if clearfirst {
		store.DeleteThread(ctx, conf.GetThreadID())
	}
	slog.Info("STORE ENABLED")
	global.SetStore(store)
	global.SetConf(*conf)

	global.AddCloser(func() error {
		os.Exit(1)
		return nil
	})

	spinner.NoSpin = conf.Get().NoSpin
	err = runmain(ctx, conf)
	if err != nil {
		return stackerr.Errorf("runmain: %w", err)
	}
	slog.Info("done without errors")
	return nil
}

const showChar = false

type codeargs struct {
	Code string `json:"code"`
}

func (c codeargs) IsToolArgument() {}

// rumain already initialized
func runmain(ctx context.Context, conf *global.Conf) error {
	if showChar {
		fmt.Fprintf(os.Stderr, "Press ctrl+d to send your message, press ctrl+c to cancel generation. slash commands are supported, see '/help'.\n")
	}

	threadID := conf.GetThreadID()

	singleeditor := implv2.NewEditTool(nil) // single file editor

	var err error
	if true { // builtin go tools
		slog.Debug("loading native tools")
		builtinTools, err = builtins.LoadBuiltinTools(map[string]otool.ExecFunc{
			"read_file":      implv1.ReadFile,
			"list_directory": implv1.ListDirectory,
			//"edit_file":      implv2.EditTool,
			"edit_file": singleeditor.EditTool,
			//"write_file": implv1.WriteFile,
			//	"run_go_command":   noop,
			"run_bash": basher1.Basher,
			//	"get_package_info": noop,
		})
		if err != nil {
			log.Fatalf("could not load builtin tools: %+v", err)
		}
		if len(builtinTools) == 0 {
			panic("no builtin tools")
		}
	}
	if true { // runtime bash/python tools
		slog.Debug("loading toolbox tools")
		builtinTools = nil
		box, err := toolboxes.New("") // use env with toolboxes subdir
		if err != nil {
			return err
		}
		tools := box.AllTools()
		for _, v := range tools {
			if !slices.Contains(builtinTools.Names(), v.GetName()) {
				builtinTools = append(builtinTools, v)
			}
		}
	}

	if true { // run tooltest
		i := slices.Index(builtinTools.Names(), "tooltest")
		if i < 0 {
			return fmt.Errorf("could not find tooltest")
		}
		caller := llm.Message{}
		x, err := builtinTools[i].Exec(ctx, caller, 0)
		if err != nil || len(x) == 0 || strings.Contains(x[0].Content, "Error") {
			err = errors.Join(err, stackerr.Errorf("tooltest failed: %q", x.Contents()))
			return fmt.Errorf("got err: %w", err)
		}
		slog.Info("tool pass", "x", x.Contents())

	}

	var stack chatmessages.A
	store := global.GetStore[*dbstore.Store]()
	if store == nil {
		slog.Warn("no store")
	}
	if store != nil {
		stack, err = store.LoadThread(ctx, conf.GetThreadID(), 4)
		if err != nil {
			return err
		}
	}

	if _, err := os.Stdin.Read(nil); err != nil {
		return stackerr.Errorf("reading stdin: %w", err)
	}
	{ // USER MESSAGE
		newmsg := chatmessages.NewFromRole(conf.GetUsername(), "user", "")
		newmsg.ThreadID = threadID
		stack = append(stack, *newmsg)
		newmsg = &stack[len(stack)-1] // fix ptr
		// read stdin directly to the newmsg
		if showChar {
			_, _ = fmt.Fprintf(os.Stderr, "%s: ", titlecaser.String(conf.GetUsername())) // eg: Chief:
		}
		err = ReadMessage(os.Stdin, conf.GetUsername(), "user", &newmsg) // ptr of ptr
		if err != nil {
			return stackerr.Errorf("ReadMessage: %w", err)
		}
		if newmsg == nil || newmsg.Content == "" {
			return nil
		}
		if err = store.Add(ctx, threadID, newmsg); err != nil {
			slog.Warn("err", "err", err)
		}
	}

	var enc *json.Encoder
	if conf.Get().JsonOutput {
		enc = json.NewEncoder(os.Stdout)
	}
	cl := debugging.NewConsoleLogger(os.Stdout, os.Stderr)
	cl.HideThinking = !conf.Get().ShowThinking
	// GENERATE ROUNDS
	e := chat.Ext{
		PreTools: func(ctx context.Context, newcaller *chatmessages.ChatMessage, names []string) error {
			for i := range newcaller.ToolCalls {
				if err := implv2.ConfirmAction(ctx, fmt.Sprintf("Requested tool call: %q\n\n```json\n%s```", names[i], string(newcaller.ToolCalls[i]))); err != nil {
					return err
				}
			}
			return nil
		},
		ThreadID: threadID,
		NonStreamHandler: func(cm chatmessages.ChatMessage) {
			if store != nil {
				if err = store.Add(ctx, threadID, &cm); err != nil {
					slog.Warn("err", "err", err)
				}
			}
			if enc != nil {
				_ = enc.Encode(cm)
			}
		}, AfterGen: func(cm *chatmessages.ChatMessage) error {
			if store != nil {
				if err = store.Add(ctx, threadID, cm); err != nil {
					return err
				}
			}
			if enc != nil {
				_ = enc.Encode(cm)
			}
			return nil
		},
		Handlers: chat.Handlers{cl.HandleChatResponse},
	}
	err = cmdpkg.ChatWith(ctx, stack, builtinTools, e)
	if err != nil {
		return stackerr.Errorf("%w", err)
	}

	return nil
}

var titlecaser = cases.Title(language.AmericanEnglish)

// ReadMessage may make newmsg nil
func ReadMessage(r io.Reader, from, role string, newmsg **chatmessages.ChatMessage) error {
	var x string
	var err error
	defer func() { slog.Info("captured input", "text", x, "err", err) }()
	// prompt
	// read stdin until EOF
	var buf []byte
	buf, err = io.ReadAll(r)
	x = strings.TrimSpace(string(buf))
	if x != "" && errors.Is(err, io.EOF) {
		err = nil
	}
	if err != nil {
		return err
	}

	//
	if *newmsg == nil {
		slog.Info("creating new pending gen message")
		*newmsg = chatmessages.NewFromRole(from, role, x)
		return nil
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
