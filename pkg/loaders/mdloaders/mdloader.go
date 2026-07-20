package mdloaders

import (
	"html/template"
	"strings"
)

func FuncMap() template.FuncMap {
	return template.FuncMap{
		"hasPrefix": func(s, p string) bool {
			return strings.HasPrefix(s, p)
		},
		"hasNotPrefix": func(s, p string) bool {
			return !strings.HasPrefix(s, p)
		},
	}
}
