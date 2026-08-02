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

var (
	BashEnvironKeys   = regexp.MustCompile(`(?m)^\s*(?:export\s+)?([A-Z_][A-Z1-9_]*)\s*=`)
	PythonEnvironKeys = regexp.MustCompile(`(?:environ\.get|getenv)\s*\(\s*['"]([^'"]+)['"]`)
)
func CleanForTTS(s string) string {
	if s == "" {
		return s
	}
    // 1. Remove big chunks: YAML, HTML comments, and Code Blocks
    s = regexp.MustCompile(`(?s)^---.*?^---`).ReplaceAllString(s, "")        // YAML
    s = regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllString(s, "")          // HTML Comments
    s = regexp.MustCompile(`(?m)^(?:\`\`\`|~~~).*?(?:\n|$)[\s\S]*?(?:^```|^~~~)`).ReplaceAllString(s, "") // Code blocks

    // 2. Extract text from formatting: Links, Images, Wiki-links, Bold/Italic
    s = regexp.MustCompile(`!\[([^\]]*)\]\([^\)]+\)`).ReplaceAllString(s, "$1") // Image alt text
    s = regexp.MustCompile(`\[([^\]]+)\]\([^\)]+\)`).ReplaceAllString(s, "$1")    // Standard Links
    s = regexp.MustCompile(`\[\[([^\|\]]+)\|([^\]]+)\]\]`).ReplaceAllString(s, "$2") // Wiki alias [[link|alias]]
    s = regexp.MustCompile(`\[\[([^\]]+)\]\]`).ReplaceAllString(s, "$1")          // Simple wiki link [[link]]

    // Remove bold, italic, strikethrough markers but keep text
    s = regexp.MustCompile(`\*\*([^\*]+)\*\*|__([^_]+)__`).ReplaceAllString(s, "$1$2")
    s = regexp.MustCompile(`\*([^*]+)\*|_([^_]+)_|~~([^~]+)~~`).ReplaceAllString(s, "$1$2$3")

    // 3. Remove structural noise: Headers, List bullets, Blockquotes, HTML tags
    s = regexp.MustCompile(`(?m)^#{1,6}\s+`).ReplaceAllString(s, "") // Headers
    s = regexp.MustCompile(`(?m)^[\s]*[-*+][ \t]+\[[ xX]\]\s+`).ReplaceAllString(s, "") // Task lists
    s = regexp.MustCompile(`(?m)^[\s]*[-*+][ \t]+`).ReplaceAllString(s, "") // Unordered lists
    s = regexp.MustCompile(`(?m)^\d+\.\s+`).ReplaceAllString(s, "")    // Ordered lists
    s = regexp.MustCompile(`(?m)^>\s*`).ReplaceAllString(s, "")        // Blockquotes
    s = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(s, "")          // HTML tags

    // 4. Final clean: Keep ASCII + Newlines/Tabs only (removes emojis/special chars)
    var sb strings.Builder
    for _, r := range s {
        if r < 128 || r == '\n' || r == '\t' || r == '\r' {
            sb.WriteRune(r)
        }
    }
    s = sb.String()

    // 5. Clean whitespace and URLs
    s = regexp.MustCompile(`https?://[^\s]+`).ReplaceAllString(s, "") // Remove remaining URLs
    s = regexp.MustCompile(`\n{3,}`).ReplaceAllString(s, "\n\n")      // Max 2 newlines
    s = strings.TrimSpace(s)

    return s
}
