package orchard

import (
	"embed"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

//go:embed *.md prompts/*.md doc/*.md
var embedded embed.FS

// Embedded markdown files (SYSTEM.md template is used as sticky system instruction)
var Embedded fs.FS = initFS() // embedded

func initFS() fs.FS {
	x := os.Getenv("ORCHARD_DIR")
	if x == "" {
		x = os.ExpandEnv("$HOME/src/orchard")
	}
	if _, err := os.Stat(filepath.Join(x, "SYSTEM.md")); err == nil {
		if debug2 {
			slog.Info("using file-system instead of embedded Orchard prompts", "orchard_dir", x)
		}
		return os.DirFS(x)
	}
	return embedded
}

var debug2 = false
