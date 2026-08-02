package orchard

import (
	"embed"
	"io/fs"
	"log/slog"
	"os"
)

//go:embed *.md prompts/*.md doc/*.md
var embedded embed.FS

// Embedded markdown files (SYSTEM.md template is used as sticky system instruction)
var Embedded fs.FS = embedded

func init() {
	if true {
		x := os.Getenv("ORCHARD_DIR")
		if x == "" {
			x = os.ExpandEnv("$HOME/src/orchard")
		}
		if _, err := os.Stat(x); err == nil {
			slog.Info("using file-system instead of embedded prompts", "orchard_dir", x)
			Embedded = os.DirFS(x)
		}
	}
}
