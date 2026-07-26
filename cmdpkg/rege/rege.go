// Package rege regex
package rege

import "regexp"

// imageAttachmentRegex = regexp.MustCompile(`\[/[^\]]+\.(jpg|png)\]`)
// var imageAttachmentRegex = regexp.MustCompile(`\[/?[^\]]+\.(?i)(jpg|jpeg|png)\]`)
var imageAttachmentRegex = regexp.MustCompile(`\[/?([^\]]+\.(?i)(jpg|jpeg|png))\]`)

// FindImagePaths searches for strings matching patterns like [/path/to/file.jpg] or [/path/to/file.png]
func FindImagePaths(s string) []string {
	matches := imageAttachmentRegex.FindAllStringSubmatch(s, -1)
	paths := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			paths = append(paths, match[1])
		}
	}
	return paths
}

var BashEnvironKeys = regexp.MustCompile(`(?m)^\s*(?:export\s+)?([A-Z_][A-Z0-9_]*)\s*=`)
