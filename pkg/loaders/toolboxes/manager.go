package toolboxes

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"m/pkg/chatmessages"
)

type (
	Params []Param
	Param  struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
		Type        string `json:"type,omitempty"`
		IsRequired  bool   `json:"required,omitempty"` // added to required list
	}
)

func (p Params) Names() []string {
	if len(p) == 0 {
		return nil
	}
	a := make([]string, len(p))
	for i, v := range p {
		a[i] = v.Name
	}
	return a
}

type CallerT struct {
	Name      string
	Arguments JSONObject
}
type JSONObject map[string]json.RawMessage

func (j JSONObject) Arg(name string) (any, bool) {
	return argsGet(j, name)
}

func (j JSONObject) ArgFloat(name string) (float64, bool) {
	x, ok := argsGet(j, name)
	if !ok {
		return 0, false
	}
	s, ok := x.(float64)
	return s, ok
}

func (j JSONObject) ArgString(name string) (string, bool) {
	x, ok := argsGet(j, name)
	if !ok {
		return "", false
	}
	s, ok := x.(string)
	return s, ok
}

type Arg struct {
	Name  string
	Value string
}

func argsGet(args map[string]json.RawMessage, name string) (any, bool) {
	var x any
	if err := json.Unmarshal(args[name], &x); err != nil {
		return nil, false
	}
	slog.Info("got arg: ", "name", name, "val", x)
	return x, true
}

// [orch.OrchTool.Exec]
func makeExecFn(toolName string, params Params, doInputEnv bool, pyFilePath string) func(ctx context.Context, caller CallerT) (chatmessages.ChatMessages, error) {
	execFn := func(ctx context.Context, caller CallerT) (chatmessages.ChatMessages, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		args := caller.Arguments
		var execArgs []string
		var inputFile string
		for i, param := range params {
			val, ok := argsGet(args, param.Name)
			//		if !ok {
			//			val, ok = args.GetStringCaseInsensitive(param.Name)
			//		}
			if !ok && param.IsRequired {
				panic("is required")
				//			return nil, fmt.Errorf("missing required param %q for tool %q (case-sensitive) - Available params: %q - Received bad param name: %q", param.Name,
				//				toolName, params.Names(), slices.Collect(maps.Keys(args.ToMap())))
			}
			if !ok {
				execArgs = append(execArgs, "") // still ordered
				continue
			}
			if doInputEnv && inputFile == "" && i == len(params)-1 {
				slog.Warn("inserting file")
				inputFile = val.(string)
			} else {
				execArgs = append(execArgs, fmt.Sprint(val))
			}
		}
		var inputfilename string
		if inputFile != "" {
			file, err := os.CreateTemp("", "input_*")
			if err != nil {
				return nil, fmt.Errorf("invalid: %w", err)
			}
			slog.Info("creating input file...", "size", len(inputFile), "name", file.Name())
			_, err = file.WriteString(inputFile)
			_ = file.Close()
			if err != nil {
				return nil, fmt.Errorf("invalid: %w", err)
			}
			inputfilename = file.Name()
		}
		_ = os.Setenv("INPUT_FILE", inputfilename)
		var outputMessage string
		var err error
		if strings.HasSuffix(pyFilePath, ".py") {
			slog.Info("executing python", "file", pyFilePath, "args", execArgs)
			//		outputMessage, err = pytooler.RunPy(ctx, ".", pyFilePath, execArgs...)
		} else if strings.HasSuffix(pyFilePath, ".bash") {
			slog.Info("executing bash", "file", pyFilePath, "args", execArgs)
			//		outputMessage, err = bashtooler.RunBash(ctx, ".", pyFilePath, execArgs...)
		}
		if err != nil {
			return chatmessages.ChatMessages{*chatmessages.NewChatMessage("tool", fmt.Sprintf("Error running tool script %q: %s", pyFilePath, err.Error()))}, nil
		}
		return chatmessages.ChatMessages{*chatmessages.NewChatMessage("tool", outputMessage)}, nil
	}
	return execFn
}
