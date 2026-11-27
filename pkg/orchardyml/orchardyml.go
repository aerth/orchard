// Package orchardyml to parse orchard.yml in the project directory
package orchardyml

import (
	"errors"
	"maps"
	"os"

	"github.com/aerth/mostly/stackerr"
	"github.com/aerth/orchard"
	"github.com/goccy/go-yaml"
)

type Y struct {
	ToolHooks map[string]string // may contain $TOOL_NAME $TOOL_ARGS
	Env       map[string]string
	Args      []string
}

func LocateParse() (Y, error) {
	name := "./orchard.yml"
	b, err := os.ReadFile(name)
	if errors.Is(err, os.ErrNotExist) {
		return Y{}, err
	}
	if err != nil {
		//	slog.Error("could not read config file", "err", err)
		return Y{}, stackerr.Errorf("read: %w", err)
	}
	y, err := Parse(b)
	if err != nil {
		//	slog.Error("could not parse after locating", "err", err)
		return y, stackerr.Errorf("parse: %w", err)
	}
	return y, nil
}

func Parse(b []byte) (Y, error) {
	y := Y{
		ToolHooks: map[string]string{},
		Env:       map[string]string{},
	}
	err := yaml.Unmarshal(b, &y)
	_, hasAll := y.ToolHooks["all"]
	_, hasStar := y.ToolHooks["*"]
	if hasAll && hasStar {
		return y, ErrHasAll
	}
	if hasAll {
		y.ToolHooks["*"] = y.ToolHooks["all"]
		delete(y.ToolHooks, "all")
	}
	return y, err
}

var ErrHasAll = errors.New("err has all")

// Load should be called *before* the first Flag() function.
//
// Modifies os.Args, env, and tool-hooks.
func Load() error {
	if os.Getenv("ORCHARD_CONFIG") != "none" { // load ./orchard.yml for project-specific env/args/hooks
		yopt, err := LocateParse()
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		//	println("found", len(yopt.ToolHooks), "hooks")
		//	for k, v := range yopt.ToolHooks {
		//		println(k+":", v)
		//	}
		maps.Copy(orchard.Custom.PostToolHookCommands, yopt.ToolHooks)
		os.Args = append(os.Args, yopt.Args...)
		for k, v := range yopt.Env {
			if err := os.Setenv(k, v); err != nil {
				return err
			}
		}
	}
	return nil
}
