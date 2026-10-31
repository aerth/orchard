package toolboxes

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	"m/orchard/pkg/builtins/implv1/bashtooler"
	"m/orchard/pkg/llm"
	"m/orchard/pkg/types/chatmessages"

	"github.com/aerth/mostly/mllm"
)

type ExecFunc = func(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error)

type StaticExecFuncFS struct {
	Name       string // eg: tooltest or read_file
	readFile   func(string) ([]byte, error)
	params     OrchParams
	doInputEnv bool
	required   []string
	pyFilePath string
}

// makeExecFn for [otool.Tool.Exec]
func makeExecFn(toolName string, params OrchParams, doInputEnv bool, readFile func(string) ([]byte, error), pyFilePath string) ExecFunc {
	_ = toolName
	var required []string
	for i := range params {
		if params[i].IsRequired {
			required = append(required, params[i].Name)
		}
	}
	s := StaticExecFuncFS{Name: toolName, readFile: readFile, required: required, doInputEnv: doInputEnv, pyFilePath: pyFilePath, params: params}
	return s.Exec
}

// Exec for [otool.Tool.Exec]
func (s StaticExecFuncFS) Exec(ctx context.Context, caller1 llm.Message, callIndex int) (chatmessages.ChatMessages, error) {
	t1 := time.Now()
	defer func() {
		slog.Info("done running tool", "function", s.Name, "duration", time.Since(t1))
	}()
	if ctx.Err() != nil {
		slog.Warn("context is finished before tool could run", "err", ctx.Err(), "cause", context.Cause(ctx))
		return nil, ctx.Err()
	}
	var caller mllm.Call[map[string]json.RawMessage]
	if len(s.required) == 0 && len(caller1.ToolCalls) == 0 {
		// to make testing easy, we can call a no-arg tooltest without any real tool calls
	} else {
		err := json.Unmarshal(caller1.ToolCalls[callIndex], &caller)
		if err != nil {
			slog.Error("check", "raw", caller1.ToolCalls[callIndex])
			return nil, fmt.Errorf("parse tool call: %w", err)
		}
	}

	if len(caller1.ToolCalls) != 0 && caller.Function.Name != s.Name {
		slog.Warn("does not equal", "x", caller.Function.Name, "want", s.Name, "caller", caller1)
		for i := range caller1.ToolCalls {
			slog.Warn("debug:\n" + string(caller1.ToolCalls[i]))
		}
		return nil, fmt.Errorf("Tool %q is currently unavailable", s.Name)
	}
	slog.Info("RUN TOOL", "function", caller.Function.Name, "args", caller.Function.Arguments)
	args := caller.Function.Arguments
	var execArgs []string
	var inputFileContents string
	if len(s.params) == 0 && len(args) != 0 {
		slog.Warn("no params for this func but args given", "nargs", len(args))
		return nil, fmt.Errorf("args given (%q) but none are allowed for this function", args)
	}
	for i, param := range s.params {
		if param.Name == "" {
			return nil, fmt.Errorf("param has no name")
		}
		val, ok := argsGet(args, param.Name)
		if !ok && slices.Contains(param.Required, param.Name) {
			return nil, fmt.Errorf("required param %q was missing from function call", param.Name)
		}
		if !ok {
			// optional arg was not present but is still a ordered positional arg for the script
			execArgs = append(execArgs, "")
			continue
		}
		if s.doInputEnv && inputFileContents == "" && i == len(s.params)-1 && param.Type[0] == "string" {
			s, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("val is not a string (is %T)", val)
			}
			inputFileContents = s
			slog.Warn("inserting file", "bytes", len(s))
			execArgs = append(execArgs, "")
		} else {
			execArgs = append(execArgs, fmt.Sprint(val))
		}
	}
	var inputfilename string
	if inputFileContents != "" { // for when inputs are too big
		file, err := os.CreateTemp("", "input_*")
		if err != nil {
			return nil, fmt.Errorf("invalid: %w", err)
		}
		slog.Info("creating input file...", "size", len(inputFileContents), "name", file.Name())
		_, err = file.WriteString(inputFileContents)
		_ = file.Close()
		if err != nil {
			return nil, fmt.Errorf("invalid: %w", err)
		}
		inputfilename = file.Name()
		_ = os.Setenv("INPUT_FILE", inputfilename)
		// TODO: defer remove file
	} else if os.Getenv("INPUT_FILE") != "" {
		_ = os.Unsetenv("INPUT_FILE")
	}
	var outputMessage string
	var err error
	pyFilePath := s.pyFilePath
	if strings.HasSuffix(pyFilePath, ".py") {
		slog.Info("executing python", "file", pyFilePath, "args", execArgs)
		//		return nil, fmt.Errorf("uhoh, we disabled python")
		outputMessage, err = bashtooler.RunPy(ctx, ".", s.readFile, pyFilePath, execArgs...)
	} else if strings.HasSuffix(pyFilePath, ".bash") {
		slog.Info("executing bash", "file", pyFilePath, "args", execArgs)
		outputMessage, err = bashtooler.RunBash(ctx, ".", s.readFile, pyFilePath, execArgs...)
	} else {
		slog.Warn("wtf?", "path", pyFilePath)
		outputMessage = "wtf tool was that? dunno"
	}
	if err != nil {
		return chatmessages.ChatMessages{*chatmessages.NewChatMessage("tool", fmt.Sprintf("Error running tool script %q: %s", pyFilePath, err.Error()))}, nil
	}
	return chatmessages.ChatMessages{*chatmessages.NewChatMessage("tool", outputMessage)}, nil
}
