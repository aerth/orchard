package treesitters

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/aerth/mostly/stackerr"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/css"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/html"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

func New(lang string) (*sitter.Parser, error) {
	parser := sitter.NewParser()
	l, ok := langs[lang]
	if !ok {
		l, _, err := Getlang(lang)
		if err != nil {
			return nil, stackerr.Errorf("cant treesitter for %q lang: %w", lang, err)
		}
		langs[lang] = l
	}
	parser.SetLanguage(l)
	return parser, nil
}

var langs = map[string]*sitter.Language{}

func MustGetlang(s string) *sitter.Language {
	l, name, err := Getlang(s)
	if err != nil {
		panic(err.Error())
	}
	_ = name
	return l
}

func Getlang(s string) (*sitter.Language, string, error) {
	ss := strings.Split(s, ".")
	if !strings.Contains(s, ".") {
	}
	slog.Warn("checking", "s", ss)
	ext := ss[len(ss)-1]
	switch ext {
	case "go":
		return golang.GetLanguage(), ext, nil
	case "py":
		return python.GetLanguage(), ext, nil
	case "html":
		return html.GetLanguage(), ext, nil
	case "ts":
		return typescript.GetLanguage(), ext, nil
	case "js":
		return javascript.GetLanguage(), ext, nil
	case "css":
		return css.GetLanguage(), ext, nil
	default:
		return nil, ext, fmt.Errorf("not implemented yet")
	}
}
