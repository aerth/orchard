package orchard

import "embed"

//go:embed *.md */*.md
var embedded embed.FS

// Embedded markdown files (SYSTEM.md template is used as sticky system instruction)
var Embedded = embedded
