package toolboxes

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/aerth/mostly/godotenv"
	"github.com/aerth/mostly/mllm"
	"github.com/aerth/orchard/pkg/bashtooler"
)

var Options = struct {
	ToolDirEnv bool
}{
	ToolDirEnv: true,
}

// type ExecFunc = func(ctx context.Context, caller mllm.Message, toolCallIndex int) (mllm.ChatMessages, error)
type (
	ExecFunc         = mllm.CallFuncV1B
	StaticExecFuncFS struct {
		Name       string // eg: tooltest or read_file
		readFile   func(string) ([]byte, error)
		params     OrchParams
		doInputEnv bool
		required   []string
		pyFilePath string
	}
)

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
	return s.ExecV1
}

/*func (s StaticExecFuncFS) Exec(ctx context.Context, caller1 mllm.Message, callIndex int) (mllm.ChatMessages, error) {
	outputMessage, err := s.ExecV1(ctx, caller1)
	return mllm.ChatMessages{*mllm.NewChatMessage(
		caller.ReplyTool(outputMessage), "", uuid.Nil())}, nil
}*/

// Exec for [otool.Tool.Exec]
func (s StaticExecFuncFS) ExecV1(ctx context.Context, args json.RawMessage) (any, error) {
	caller1, _ := mllm.CtxCaller(ctx)
	callIndex, _ := mllm.CtxCallIndex(ctx)
	t1 := time.Now()
	defer func() {
		if d := time.Since(t1); d > time.Second {
			slog.Info("done running tool", "function", s.Name, "duration", d.String())
		}
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

	if mllm.DebugLLM {
		slog.Info("RUN TOOL", "function", caller.Function.Name, "args", caller.Function.Arguments)
	}
	var execArgs []string
	var inputFileContents string
	if len(s.params) == 0 && len(args) > 4 {
		slog.Warn("no params for this func but args given", "nargs", len(args))
		return nil, fmt.Errorf("args given (%q) but none are allowed for this function", args)
	}
	argsmap := map[string]json.RawMessage{}
	if err := json.Unmarshal(args, &argsmap); err != nil {
		return nil, err
	}
	for i, param := range s.params {
		if param.Name == "" {
			return nil, fmt.Errorf("param has no name")
		}
		val, ok := argsGet(argsmap, param.Name)
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
	var eenv []string
	if Options.ToolDirEnv { // otools/toolboxes/web/web_search.bash script should load otools/toolboxes/web/.env or otools/.env (or both o_O)
		envname := filepath.Join(filepath.Dir(s.pyFilePath), ".env")
		if _, err := os.Stat(envname); err != nil {
			// skip box name, skip 'toolboxes'
			envname = filepath.Join(filepath.Dir(s.pyFilePath), "../../.env")
		}
		if _, err := os.Stat(envname); err == nil {
			envmap, _ := godotenv.Read(envname) // toolboxes/boxname
			if len(envmap) != 0 {
				i := 0
				eenv = make([]string, len(envmap))
				for k, v := range envmap {
					eenv[i] = k + "=" + v
					i++
				}
			}
		}
	}
	if strings.HasSuffix(pyFilePath, ".py") {
		slog.Info("executing python for toolbox tool", "file", filepath.Base(pyFilePath), "args", execArgs)
		//		return nil, fmt.Errorf("uhoh, we disabled python")
		outputMessage, err = bashtooler.RunPy(ctx, ".", eenv, s.readFile, pyFilePath, execArgs...)
	} else if strings.HasSuffix(pyFilePath, ".bash") {
		slog.Info("executing bash for toolbox tool", "file", filepath.Base(pyFilePath), "args", execArgs)
		outputMessage, err = bashtooler.RunBash(ctx, ".", eenv, s.readFile, pyFilePath, execArgs...)
	} else {
		slog.Warn("wtf?", "path", pyFilePath)
		outputMessage = "wtf tool was that? dunno"
	}
	if err != nil {
		outputMessage += fmt.Sprintf("Error running tool script %q: %s", pyFilePath, err.Error())
	}
	return outputMessage, nil
}
