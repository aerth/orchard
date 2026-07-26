package implv2

import (
	"testing"

	"github.com/smacker/go-tree-sitter/golang"
)

func TestLint(t *testing.T) {
	s := "package main\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n"
	lang := "go"
	err := ValidateFileSyntax(s, lang, golang.GetLanguage())
	if err != nil {
		t.Fatal(err)
	}
}
