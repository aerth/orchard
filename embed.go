package orchard

import (
	"embed"
	"io/fs"
)

//go:embed *.md prompts/*.md doc/*.md
var embedded embed.FS

// Embedded markdown files (SYSTEM.md template is used as sticky system instruction)
var Embedded fs.FS = embedded

func init() {
	if true {
		Embedded
	}
}
