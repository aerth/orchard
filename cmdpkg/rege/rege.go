// Package rege regex
package rege

import "regexp"

//imageAttachmentRegex = regexp.MustCompile(`\[/[^\]]+\.(jpg|png)\]`)
var imageAttachmentRegex = regexp.MustCompile(`\[/[^\]]+\.(?i)(jpg|jpeg|png)\]`)

// FindImagePaths searches for strings matching patterns like [/path/to/file.jpg] or [/path/to/file.png]
func FindImagePaths(s string) []string {
	return imageAttachmentRegex.FindAllString(s, -1)
}
