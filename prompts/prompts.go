// Package prompts embedded
package prompts

import (
	"embed"
	_ "embed"
	"log/slog"
)

//go:embed role.bashguard.txt
var bashguard string

//go:embed role.compactor.txt
var compactor string

//go:embed system.v1.md
var system string

//go:embed *.md *.txt
var FS embed.FS

func Get(name string) (string, bool) {
	if name == "bashguard" {
		return bashguard, true
	}
	if name == "system" {
		return system, true
	}
	if name == "compactor" {
		return compactor, true
	}
	b, err := FS.ReadFile(name)
	if err != nil {
		slog.Error("read embedded file %q: %v", name, err)
		return "", false
	}
	return string(b), true
}
