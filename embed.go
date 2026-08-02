package orchard

import (
	"embed"
	"io/fs"
	"os"
)

//go:embed *.md prompts/*.md doc/*.md
var embedded embed.FS

// Embedded markdown files (SYSTEM.md template is used as sticky system instruction)
var Embedded fs.FS = embedded

func init() {
	if true {
		Embedded = os.DirFS(os.ExpandEnv("$HOME/src/orchard"))
	}
}
