// Package prompts embedded
package prompts

import (
	"embed"
	_ "embed"
)

//go:embed role.bashguard.txt
var bashguard string

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
	return "", false
}
