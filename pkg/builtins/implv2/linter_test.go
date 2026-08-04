package implv2

import (
	"testing"

	"m/pkg/loaders/treesitters"
)

func TestLint(t *testing.T) {
	s := "package main\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n"
	lang := "go"
	err := ValidateFileSyntax(s, lang, treesitters.MustGetlang("go"))
	if err != nil {
		t.Fatal(err)
	}
}
