package orchard

import "embed"

//go:embed *.md */*.md
var embedded embed.FS
var Embedded = embedded
