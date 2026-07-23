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

	"m/pkg/builtins/implv1/bashtooler"
	"m/pkg/chatmessages"
	"m/pkg/llm"
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

type Call struct {
	ID       string `json:"id"`
	Function struct {
		Index     int        `json:"index"`
		Name      string     `json:"name"`
		Arguments JSONObject `json:"arguments"`
	} `json:"function"`
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

// Manager holds tools by name, all with exec funcs attached
// type Manager map[string]Tool

//func (t Tool) MarshalJSON() ([]byte, error) {
//	type Alias Tool
//	return json.Marshal(Alias(t))
//}

func New(toolboxdir string) (*ToolboxManager, error) {
	if toolboxdir == "" {
		toolboxdir = os.ExpandEnv(os.Getenv("ORCHARD_TOOLS"))
		if toolboxdir == "" {
			return nil, fmt.Errorf("no ORCHARD_TOOLS dir")
		}
		toolboxdir = filepath.Join(toolboxdir, "toolboxes")
	}
	got, err := NewToolboxFS(toolboxdir)
	return got, err
}

const ToolboxManagerName = "toolbox_open"

type ToolboxManager struct {
	Toolboxes       Toolboxes
	Toolbox         *Toolbox
	AdditionalTools Tools
	dir             string
}

func (t *ToolboxManager) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.Toolbox.ToolSpecs())
}

type ExecFunc = func(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error)

// [orch.OrchTool.Exec]
func makeExecFn(toolName string, params OrchParams, doInputEnv bool, pyFilePath string) ExecFunc {
	_ = toolName
	execFn := func(ctx context.Context, caller1 llm.Message, callIndex int) (chatmessages.ChatMessages, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var caller chatmessages.Call[map[string]json.RawMessage]
		err := json.Unmarshal(caller1.ToolCalls[callIndex], &caller)
		if err != nil {
			return nil, err
		}
		args := caller.Function.Arguments
		var execArgs []string
		var inputFile string
		for i, param := range params {
			if param.Name == "" {
				return nil, fmt.Errorf("param has no name")
			}
			val, ok := argsGet(args, param.Name)
			//		if !ok {
			//			val, ok = args.GetStringCaseInsensitive(param.Name)
			//		}
			if !ok && slices.Contains(param.Required, param.Name) {
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
		if strings.HasSuffix(pyFilePath, ".py") {
			slog.Info("executing python", "file", pyFilePath, "args", execArgs)
			return nil, fmt.Errorf("uhoh, we disabled python")
			//		outputMessage, err = pytooler.RunPy(ctx, ".", pyFilePath, execArgs...)
		} else if strings.HasSuffix(pyFilePath, ".bash") {
			slog.Info("executing bash", "file", pyFilePath, "args", execArgs)
			outputMessage, err = bashtooler.RunBash(ctx, ".", pyFilePath, execArgs...)
		}
		if err != nil {
			return chatmessages.ChatMessages{*chatmessages.NewChatMessage("tool", fmt.Sprintf("Error running tool script %q: %s", pyFilePath, err.Error()))}, nil
		}
		return chatmessages.ChatMessages{*chatmessages.NewChatMessage("tool", outputMessage)}, nil
	}
	return execFn
}

func (t *ToolboxManager) AllTools() Tools {
	var a Tools
	for i := range t.Toolboxes {
		a = append(a, t.Toolboxes[i].ToolSpecs()...)
	}
	a = append(a, t.AdditionalTools...)
	a = append(a, t.ToolSpec())
	return a
}

func (t *ToolboxManager) CountAllTools() int {
	n := 0
	for i := range t.Toolboxes {
		n += len(t.Toolboxes[i].Tools)
	}
	return n
}

func (t *ToolboxManager) GetName() string {
	return ToolboxManagerName
}

func (t *ToolboxManager) GetDescription() string {
	return "Open a toolbox that contains more tools."
}
