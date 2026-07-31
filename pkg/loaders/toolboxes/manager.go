package toolboxes

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
	slog.Info("toolboxman marshal to json")
	return json.Marshal(t.Toolbox.ToolSpecs())
}
