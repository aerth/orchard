// Package builtins contains our AI Tools that are implemented in Go
package builtins

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"

	"m/pkg/otool"
)

//go:embed *.json
var Embedded embed.FS

// LoadTools reads all .json files from the embedded FS and initializes Tool objects.
// You pass in a map of actual Go functions to bind them.
func LoadTools(execs map[string]otool.ExecFunc) (otool.Tools, error) {
	var tools otool.Tools

	err := fs.WalkDir(Embedded, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Only process .json files
		if !d.IsDir() && filepath.Ext(path) == ".json" {
			data, err := Embedded.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read %s: %w", path, err)
			}

			var newtools otool.Tools
			// This uses your custom UnmarshalJSON which extracts the Name and Spec
			if err := json.Unmarshal(data, &newtools); err != nil {
				var tool otool.Tool
				if err := json.Unmarshal(data, &tool); err == nil {
					newtools = otool.Tools{tool}
				} else {
					return fmt.Errorf("failed to unmarshal %s: %w", path, err)
				}
			}

			// Bind the actual Go function based on the name found in JSON
			for _, tool := range newtools {
				if fn, ok := execs[tool.Name]; ok {
					tool.SetExec(fn)
					tools = append(tools, tool)
				} else {
					//				return fmt.Errorf("no implementation found for tool: %s", tool.Name)
					slog.Warn("no implementation found for tool, skipping", "tool", tool.Name)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return tools, nil
}
