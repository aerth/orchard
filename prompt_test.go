package orchard_test

import (
	"os"
	"strings"
	"testing"
	"text/template"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/orchard"
)

func must1[T any](v T, err error) T {
	if err != nil {
		panic(err.Error())
	}
	return v
}

func TestPrompt(t *testing.T) {
	b := must1(os.ReadFile("prompts/system.v1.md"))
	tpl := template.Must(template.New("").Parse(string(b)))
	_ = tpl
	t.Run("no tools", func(t *testing.T) {
		ocfg := orchard.Config{}
		got, err := ocfg.DefaultSystemPrompt(orchard.StateConfig{})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(got, "tool names") {
			println(got)
			t.Fatal("unexpected tool names")
		}
	})
	t.Run("with tool name", func(t *testing.T) {
		ocfg := orchard.Config{
			Toolbox: []mllm.Tool{{Spec: mllm.Spec(`{"name": "dd"}`)}},
		}
		got, err := ocfg.DefaultSystemPrompt(orchard.StateConfig{})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, `Available tool names: ["dd"]`) {
			println(got)
			t.Fatal("expected tool name to be in prompt")
		}
	})
}
