package orchard

import (
	"os"
	"testing"
	"text/template"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err.Error())
	}
	return v
}

func TestPrompt(t *testing.T) {
	b := must(os.ReadFile("testdata/testprompt.md"))
	tpl := template.Must(template.New("").Parse(string(b)))
	if err := tpl.Execute(os.Stdout, templateData{}); err != nil {
		t.Fatal(err)
	}
}
