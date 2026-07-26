// Package toolboxes ...
package toolboxes

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"m/cmdpkg/rege"
	"m/pkg/chatmessages"
	"m/pkg/llm"
	"m/pkg/otool"
	"m/pkg/yoke"
)

type Toolbox struct {
	Name    string
	Ordered []string // optional
	Tools   map[string]AnyTool
}

type AnyTool interface {
	GetName() string
	GetDescription() string
	json.Marshaler
}
type Toolboxes []Toolbox

func (t Toolboxes) NamesAny() []any {
	a := make([]any, len(t))
	for i := range a {
		a[i] = t[i].Name
	}
	return a
}

func (t Toolboxes) Names() []string {
	a := make([]string, len(t))
	for i := range a {
		a[i] = t[i].Name
	}
	return a
}

func (t *Toolboxes) Add(t2 ...Toolbox) Toolboxes {
	*t = append(*t, t2...)
	return *t
}

/*
	func (t Toolboxes) Export() api.Tools {
		a := make(api.Tools, len(t))
		for i := range t {
			a[i] = api.Tool{
				Type: "string",
				Function: api.ToolFunction{
					Name:        t[i].Name,
					Description: t[i].GetDescription(),
				},
			}
		}
		return a
	}
*/

/*
type Tool struct {
	Type     string       `json:"type,omitempty"`
	Function ToolFunction `json:"function"`
	exec     func(ctx context.Context, caller CallerT) (chatmessages.ChatMessages, error)
}

func (t Tool) WithExec(execFn func(ctx context.Context, caller CallerT) (chatmessages.ChatMessages, error)) Tool {
	t.exec = execFn
	return t
}*/

type (
	Tools           = otool.Tools
	Tool            = otool.Tool
	ToolFunctionRaw struct {
		Name, Description string
		Parameters        any
	}
)

type ToolFunction struct {
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Parameters  OrchParamsWrapper `json:"parameters,omitempty"`
}
type OrchParamsWrapper struct {
	Type       string               `json:"type,omitempty"`
	Required   []string             `json:"required,omitempty"`
	Properties map[string]OrchParam `json:"properties,omitempty"`
}

// func (t Tool) GetName() string        { return t.Function.Name }
// func (t Tool) GetDescription() string { return t.Function.Description }
// type Tools []Tool
/*
func (t Tools) Export() []json.RawMessage {
	a := make([]json.RawMessage, len(t))
	for i := range t {
		b, err := t[i].MarshalJSON()
		if err != nil {
			panic(err.Error())
		}
		a[i] = b
	}
	return a
}
*/
func (t *Toolbox) ToolSpecs() Tools {
	var a Tools
	for _, v := range t.Tools {
		//	slog.Info("exporting tool in box", "toolbox", t.Name, "tool", v.GetName())
		if orchtool, ok := v.(Tool); ok {
			a = append(a, orchtool)
			continue
		} else {
			panic("not ready")
			continue
		}
	}
	return a
}

func (t Toolbox) Export() Tools {
	return t.ToolSpecs()
}

func (t Toolboxes) MustByName(name string) *Toolbox {
	x, ok := t.ByName(name)
	if !ok {
		panic(fmt.Sprintf("not ok: %s is not in %q", name, t.Names()))
	}
	return x
}

func (t Toolboxes) ByName(name string) (*Toolbox, bool) {
	for i := range t {
		if t[i].Name == name {
			//		slog.Warn("byname", "want", name, "checking", t[i].Name)
			return &t[i], true
		}
	}
	return nil, false
}

func (t Toolbox) GetDescriptionUnopened() string {
	return fmt.Sprintf("open %s", t.DescribeUnopened())
}

func (t Toolbox) DescribeUnopened() string {
	return fmt.Sprintf("toolbox %q: contains %d tools that are currently unavailable (%q)", t.Name, len(t.Tools), slices.Collect(t.Names()))
}

func (t Toolbox) DescribeOpened() string {
	return fmt.Sprintf("toolbox %q: contains %d tools that are currently available (%q)", t.Name, len(t.Tools), slices.Collect(t.Names()))
}

func (t Toolbox) Names() iter.Seq[string] {
	return maps.Keys(t.Tools)
}

func (t Toolbox) ByName(name string) AnyTool {
	x := t.Tools[name]
	return x
}

func GetBashFileEnvironKeys(b []byte) ([]string, error) {
	// Matches: export KEY="val", KEY='val', or KEY=val
	matches := rege.BashEnvironKeys.FindAllStringSubmatch(string(b), -1)

	keys := make([]string, 0, len(matches))
	seen := make(map[string]bool)
	for _, m := range matches {
		if !seen[m[1]] {
			keys = append(keys, m[1])
			seen[m[1]] = true
		}
	}
	return keys, nil
}

func GetPythonFileEnvironKeys(b []byte) ([]string, error) {
	// Matches words inside quotes inside .get( or getenv(
	matches := rege.PythonEnvironKeys.FindAllStringSubmatch(string(b), -1)

	keys := make([]string, len(matches))
	for i, m := range matches {
		keys[i] = m[1] // Extract the captured quote group
	}
	return keys, nil
}

const MaxToolboxes = 20

type (
	OrchParams []OrchParam
	OrchParam  struct { // a single param arg such as 'filename' string
		Name        string            `json:"-"`
		Description string            `json:"description"`
		Type        yoke.PropertyType `json:"type"`
		Properties  *yoke.Property    `json:"properties,omitempty"` // for inner
		Required    []string          `json:"required,omitempty"`   // if any
		IsRequired  bool              `json:"-"`
	}
)

func (o OrchParam) MarshalJSON() ([]byte, error) {
	var props yoke.Property
	if o.Properties != nil {
		props = *o.Properties
	}
	if props.Description == "" {
		props.Description = o.Description
	}
	if len(props.Type) == 0 {
		props.Type = o.Type
	}
	return json.Marshal(props)
}

func (o OrchParams) Export() any {
	return map[string]any{
		"properties": o.ToMap(),
		"type":       "object",
	}
}

func (o OrchParams) ToMap() map[string]OrchParam {
	m := map[string]OrchParam{}
	for i := range o {
		m[o[i].Name] = o[i]
	}
	return m
}

/*
	type OrchParam struct {
		Type        string        `json:"type"`
		Name        string        `json:"name"`
		Description string        `json:"description,omitempty"`
		IsRequired  bool          `json:"-"`          // syntactic sugar only. added to main parameters 'required' list at Export
		Props       yoke.Property `json:"properties"` // for this prop
	}
*/
var debugToolGen = os.Getenv("DEBUG_TOOL_GEN") == "1"

func parsePythonMetadata(filename string, content string) (string, OrchParams, []string) {
	var description string
	var params []OrchParam
	var requiredParamNames []string

	lines := strings.SplitSeq(content, "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "#") {
			break
		}
		if strings.HasPrefix(line, "#!") {
			continue
		}
		line = strings.TrimPrefix(line, "#")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if debugToolGen {
			slog.Info("Parsing metadata", "filename", filename, "line", line)
		}
		if after, ok := strings.CutPrefix(line, "Description:"); ok {
			if description != "" {
				description += "\n"
			}
			description += strings.TrimSpace(after)
			continue
		}
		if after, ok := strings.CutPrefix(line, "Param:"); ok {
			paramStr := strings.TrimSpace(after)
			// Format: name - description [type] [optional/required]
			splparts := strings.SplitN(paramStr, " - ", 2)
			if len(splparts) == 0 {
				slog.Warn("no param args")
				continue
			}
			paramName := strings.TrimSpace(splparts[0])
			if paramName == "" {
				slog.Warn("no param name split")
				continue
			}

			var paramDesc string
			paramType := "string"
			isRequired := true

			if len(splparts) > 1 {
				rem := strings.TrimSpace(strings.Join(splparts[1:], " "))
				// Extract type and optional/required indicators if specified
				if strings.Contains(rem, "[") { // eg: [string] [optional]
					// Extract parameter description up to the first bracket
					idx := strings.Index(rem, "[")
					paramDesc = strings.TrimSpace(rem[:idx])

					bracketContent := rem[idx:]
					if strings.Contains(bracketContent, "optional") {
						isRequired = false
					}
					if strings.Contains(bracketContent, "required") {
						isRequired = true
					}
					if strings.Contains(bracketContent, "int") {
						paramType = "integer"
					} else if strings.Contains(bracketContent, "bool") {
						paramType = "boolean"
					}
					rem = ""
				}
				if rem != "" {
					paramDesc += rem
					slog.Warn("TOOLBOX: assuming param is [string] -- add [string] to end of param description to hide this log message", "param", paramName, "remainder", rem, "full_description", paramDesc)
				}
			}
			if isRequired {
				requiredParamNames = append(requiredParamNames, paramName)
			}

			params = append(params, OrchParam{
				Name:        paramName,
				Description: paramDesc,
				Type:        yoke.PropertyType{paramType},
			})
		}
	}
	return description, params, requiredParamNames
}

func NewToolboxFSX(basepath string) (Toolboxes, error) {
	x, err := NewToolboxFS(basepath)
	if x != nil {
		return x.Toolboxes, err
	}
	return nil, err
}

// NewToolboxFS scans the filesystem under the base path and dynamically constructs Toolboxes.
// It ignores non-python files, empty directories, and python files directly in the base directory.
func NewToolboxFS(basepath string) (*ToolboxManager, error) {
	slog.Info("NewToolboxFS Reading filesystem", "basepath", basepath)
	entries, err := os.ReadDir(basepath)
	if err != nil {
		return nil, fmt.Errorf("basepath readdir: %w", err)
	}
	var tbs Toolboxes

	if len(entries) > MaxToolboxes {
		return nil, fmt.Errorf("too many toolboxes (%d) > %d", len(entries), MaxToolboxes)
	}
	log.Printf("Toolboxs: %q", entries)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		subDirName := entry.Name()
		// Ignore hidden directories like .git
		if strings.HasPrefix(subDirName, ".") {
			slog.Error("skipping dotfile", "name", subDirName)
			continue
		}

		//	slog.Info("testing dir", "entry", entry.Name(), "en", entry)
		subDirPath := filepath.Join(basepath, subDirName)
		subEntries, err := os.ReadDir(subDirPath)
		if err != nil {
			return nil, fmt.Errorf("subdir readdir: %w", err)
		}

		toolsMap := make(map[string]AnyTool)
		var orderedTools []string

		for _, subEntry := range subEntries {
			if subEntry.IsDir() || (!strings.HasSuffix(subEntry.Name(), ".py") && !strings.HasSuffix(subEntry.Name(), ".bash")) {
				continue
			}

			scriptFilename := subEntry.Name()
			toolName := strings.TrimSuffix(scriptFilename, ".py")
			toolName = strings.TrimSuffix(toolName, ".bash")
			pyFilePath := filepath.Join(subDirPath, scriptFilename)

			contentBytes, err := os.ReadFile(pyFilePath)
			if err != nil {
				continue
			}

			description, params, requiredParamNames := parsePythonMetadata(pyFilePath, string(contentBytes))
			if description == "" {
				description = "Executes " + toolName
			}
			if debugToolGen {
				log.Printf("found %d params for %q", len(params), toolName)
			}

			var envs []string
			if strings.HasSuffix(scriptFilename, ".py") {
				envs, err = GetPythonFileEnvironKeys(contentBytes)
			} else {
				envs, err = GetBashFileEnvironKeys(contentBytes)
			}
			if err != nil {
				slog.Error("env detect", "err", err)
			}
			// doInputEnv
			doInputEnv := slices.Contains(envs, "INPUT_FILE")
			execFn := makeExecFn(toolName, params, doInputEnv, pyFilePath)
			//		orchTool := MakeAPITool1(toolName, description, execFn, params...)
			par := OrchParamsWrapper{Required: requiredParamNames, Properties: params.ToMap()}
			spec := ToolFunction{Name: toolName, Description: description, Parameters: par} // params, requiredParamNames}
			specjs, _ := json.Marshal(spec)
			orchTool := Tool{
				Name: toolName,
				Spec: specjs,
			}.WithExec(execFn)
			toolsMap[toolName] = orchTool
			orderedTools = append(orderedTools, toolName)
		}

		if len(toolsMap) > 0 {
			slices.Sort(orderedTools)
			tbs = append(tbs, Toolbox{
				Name:    subDirName,
				Ordered: orderedTools,
				Tools:   toolsMap,
			})
		}
	}

	// Sort toolboxes by Name for stability
	slices.SortFunc(tbs, func(a, b Toolbox) int {
		return strings.Compare(a.Name, b.Name)
	})

	for i := range tbs {
		_ = tbs[i].Export() // crash early
	}

	var tb *Toolbox
	if len(tbs) != 0 {
		tb = &tbs[0]
	}
	return &ToolboxManager{Toolboxes: tbs, dir: basepath, Toolbox: tb}, nil
}

// Exec opens a toolbox, assigning t.Toolbox
func (t *ToolboxManager) Exec(ctx context.Context, caller llm.Message, callIndex int) (chatmessages.ChatMessages, error) {
	var call Call
	err := json.Unmarshal(caller.ToolCalls[callIndex], &call)
	if err != nil {
		return nil, err
	}
	args := call.Function.Arguments
	isToolNotMan := call.Function.Name != ToolboxManagerName
	slog.Info("executing toolbox manager", "isManagerTool", !isToolNotMan, "tool", call.Function.Name, "args", args)
	toolboxName, ok := args.ArgString("name")
	if !ok {
		return nil, fmt.Errorf("need toolbox name to open (available: %q)", t.Toolboxes.Names())
	}
	toolbox, ok := t.Toolboxes.ByName(toolboxName)
	if !ok {
		return nil, fmt.Errorf("toolbox not found by name: %s", toolboxName)
	}
	t.Toolbox = toolbox
	return RepairedToolCallIDs(ctx, call, chatmessages.ChatMessages{*chatmessages.NewChatMessage("tool", "Toolbox opened. "+toolbox.DescribeOpened())}), nil
}

func RepairedToolCallIDs(ctxWithVal context.Context, call Call, msgs chatmessages.ChatMessages) chatmessages.ChatMessages {
	funcName := call.Function.Name
	callID := call.ID
	if funcName == "" || callID == "" {
		panic(fmt.Sprintf("missing info: funcname=%q callID=%q", funcName, callID))
	}
	return RepairedToolCallIDsNew(msgs, callID, funcName)
}

func RepairedToolCallIDsNew(msgs chatmessages.ChatMessages, callID string, funcName string) chatmessages.ChatMessages {
	for i := range msgs {
		if msgs[i].Role == "tool" && msgs[i].ToolCallID == "" {
			msgs[i].ToolCallID = callID
			msgs[i].ToolName = funcName
		}
	}
	return msgs
}

func (t *ToolboxManager) ToolSpec() Tool {
	props := yoke.Property{
		Type: []string{"string"},
		Enum: t.Toolboxes.NamesAny(),
	}
	params := OrchParams{{Name: "name", Description: "name of toolbox to open", Type: yoke.PropertyType{"string"}, Properties: &props}}

	expo := params.Export()
	spec := map[string]any{
		"name": t.GetDescription(), "description": t.GetDescription(),
		"parameters": expo,
	}
	specjs, err := json.MarshalIndent(spec, "", " ")
	if err != nil {
		panic(err)
	}
	//println("SPEC", string(specjs))
	return (Tool{
		Name: t.GetName(),
		Spec: specjs,

		//	Description: t.GetDescription(),
		//	Required:        []string{"name"},
	}).WithExec(t.Exec)
}
