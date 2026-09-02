// Package builtins contains our AI Tools that are implemented in Go
package builtins

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"

	"m/orchard/pkg/llm"
	"m/orchard/pkg/types/chatmessages"
	"m/orchard/pkg/types/otool"
)

//go:embed *.json
var Embedded embed.FS

func Noop(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
	return nil, nil
}

// LoadBuiltinTools reads all .json files from the embedded FS and initializes Tool objects.
// You pass in a map of actual Go functions to bind them.
func LoadBuiltinTools(execs map[string]otool.ExecFunc) (otool.Tools, error) {
	var tools otool.Tools

	err := fs.WalkDir(Embedded, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Only process .json files
		if !d.IsDir() && filepath.Ext(path) == ".json" {
			data, err := Embedded.ReadFile(path) // ie: builtins.tools.json
			if err != nil {
				return fmt.Errorf("failed to read %s: %w", path, err)
			}

			var newtools otool.Tools
			// extracts the Name and Spec
			if err := json.Unmarshal(data, &newtools); err != nil {
				var tool otool.Tool
				if err := json.Unmarshal(data, &tool); err == nil {
					newtools = otool.Tools{tool}
				} else {
					return fmt.Errorf("failed to unmarshal %s: %w", path, err)
				}
			}

			// newtools dont have exec functions. we use this function's execs map param
			for _, tool := range newtools {
				if fn, ok := execs[tool.Name]; ok {
					tool = tool.WithExec(fn)
					tools = append(tools, tool)
					slog.Info("builtin tool", "name", tool.Name)
				} else if false {
					slog.Warn("no implementation found for builtin tool, skipping", "name", tool.Name)
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
