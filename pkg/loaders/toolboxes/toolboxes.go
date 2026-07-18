// Package toolboxes ...
package toolboxes

import (
	"encoding/json"
	"fmt"
	"iter"
	"maps"
	"regexp"
	"slices"
)

type Toolbox struct {
	Name    string
	Ordered []string // optional
	Tools   map[string]AnyTool
}

type AnyTool interface {
	GetName() string
	GetDescription() string
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

type Tool struct {
	Type     string
	Function ToolFunction
}
type ToolFunction struct {
	Name, Description string
	Parameters        map[string]json.RawMessage
}

func (t Tool) GetName() string        { return t.Function.Name }
func (t Tool) GetDescription() string { return t.Function.Description }

type Tools []Tool

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
	panic("not ready yet")
	return nil
	//return t.ToolSpecs().Export()
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

func GetPythonFileEnvironKeys(b []byte) ([]string, error) {
	// Matches words inside quotes inside .get( or getenv(
	re := regexp.MustCompile(`(?:environ\.get|getenv)\s*\(\s*['"]([^'"]+)['"]`)
	matches := re.FindAllStringSubmatch(string(b), -1)

	keys := make([]string, len(matches))
	for i, m := range matches {
		keys[i] = m[1] // Extract the captured quote group
	}
	return keys, nil
}

const MaxToolboxes = 20
