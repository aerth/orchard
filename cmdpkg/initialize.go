// Package cmdpkg for shared main-package things
package cmdpkg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"m/pkg/global"
	"m/pkg/llm"
	"m/pkg/types/chatmessages"
	"m/pkg/types/otool"

	"github.com/aerth/mostly/stackerr"
	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
)

// See Also: [global.ConfigInput.Flags]

//var okenvs = map[string]bool{
//}

/*
	func isbad(s string) bool {
		if okenvs[s] {
			return false
		}
		if strings.HasPrefix(s, "OLLAMA_") || strings.HasPrefix(s, "ORCHARD_") || strings.HasPrefix(s, "LLM_") {
			return false
		}
		return true
	}

	func NewCleanEnviron() []string {
		env := slices.DeleteFunc(os.Environ(), func(s string) bool {
			x := strings.Split(s, "=")[0]
			return isbad(x)
		})
		if !slices.ContainsFunc(env, func(e string) bool {
			return strings.HasPrefix(e, "REQUESTS_CA_BUNDLE")
		}) {
			env = append(env, "REQUESTS_CA_BUNDLE=/etc/ssl/certs/ca-certificates.crt")
		}
		//for i := range env {
		//		log.Printf("using python with env %s", env[i])
		//	}
		return env
	}

	func ResetEnvClean() {
		// clear env, then replace with clean env -- this new, cleaned env is passed to tool calls for python/bash tools
		{
			customEnv := NewCleanEnviron()
			os.Clearenv()
			for _, env := range customEnv {
				before, after, _ := strings.Cut(env, "=")
				if before == "" {
					continue
				}
				if err := os.Setenv(before, after); err != nil {
					panic(err.Error())
				}
			}
		}
	}
*/
func ActivateTools(tools otool.Tools) error {
	for i := range tools {
		if tools[i].Name == "foo" {
			tools[i] = tools[i].WithExec(
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

var ENVFILE = os.ExpandEnv("$HOME/.config/orchard/env")

func LOADENV() error {
	homedir, err := os.UserHomeDir()
	if err != nil {
		slog.Info("loading env", "file", ENVFILE)
		return err
	}
	ENVFILE = strings.Replace(ENVFILE, "~", homedir, 1)
	if os.Getenv("HOME") == "" {
		slog.Info("loading env", "file", ENVFILE)
		return fmt.Errorf("no HOME")
	}
	if err := godotenv.Load(ENVFILE); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Info("loading env", "file", ENVFILE)
		return err
	}

	// support tilde in paths in case shell did not expand
	if x := os.Getenv("ORCHARD_TREE"); x != "" {
		x = strings.Replace(x, "~", homedir, 1)
		_ = os.Setenv("ORCHARD_TREE", x)
	}
	if x := os.Getenv("ORCHARD_TOOLS"); x != "" {
		x = strings.Replace(x, "~", homedir, 1)
		_ = os.Setenv("ORCHARD_TOOLS", x)
	}
	return nil
}

func loadInitFlagsEnv(clearfirst *bool) (*global.Conf, error) {
	if err := LOADENV(); err != nil {
		return nil, err
	}
	return global.NewConfigFlags(clearfirst), nil
}

type reloadWriter struct {
	filename string
	file     *os.File
}

var _ io.Writer = reloadWriter{}

func Initialize(withSource bool, timeFormat string, clearfirst *bool) (*global.Conf, error) {
	//store := dbstore.NewFromEnv()
	conf, err := loadInitFlagsEnv(clearfirst)
	if err != nil {
		return nil, stackerr.Errorf("%w", err)
	}
	// set up logging
	// log.Printf("Got config: %#v", conf)
	if filename := conf.GetLogFilename(); filename != "" {
		f, err := os.OpenFile(filename, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			return nil, stackerr.Errorf("%w", err)
		}
		Writer = f
		global.LogWriter = f
	}
	InitLog(withSource, timeFormat).Set(slog.LevelDebug)
	//	for v := range dbstore.GormIter[chatmessages.ChatMessage](store.DB(), nil) {}
	return conf, nil
}

var loglevel *slog.LevelVar

var (
	Writer      io.Writer = os.Stderr
	ReplaceAttr func(groups []string, attr slog.Attr) slog.Attr
)

// InitLog color slog
//
// InitLog().Set(slog.LevelInfo) or use ptr for flag etc
func InitLog(withSource bool, timeFormat string, loglevel1 ...*slog.LevelVar) *slog.LevelVar {
	if len(loglevel1) == 1 {
		loglevel = loglevel1[0]
	} else if loglevel == nil {
		loglevel = new(slog.LevelVar)
		loglevel.Set(slog.LevelDebug)
	}
	if Writer == nil {
		Writer = os.Stderr
	}
	slog.SetLogLoggerLevel(slog.LevelWarn) // for now to use less
	slog.SetDefault(slog.New(tint.NewTextHandler(Writer, &tint.Options{
		AddSource: withSource, Level: loglevel,
		TimeFormat:  timeFormat,
		ReplaceAttr: ReplaceAttr, // nil
		//
		//	TimeFormat: "",
		NoColor: global.Get().Get().NoColor,
	})))
	return loglevel
}
