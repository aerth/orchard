package toolboxes_test

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/aerth/orchard/pkg/toolboxes"
)

func TestToolboxes(t *testing.T) {
	t.Chdir("../../../")
	toolboxdir := os.ExpandEnv(os.Getenv("ORCHARD_TOOLS"))
	if toolboxdir == "" {
		println("need ORCHARD_TOOLS env set to an otools directory")
		t.SkipNow()
	}
	toolboxdir = filepath.Join(toolboxdir, "toolboxes")
	log.Printf("toolbox dir: %q", toolboxdir)
	tb, err := toolboxes.NewToolboxFS(toolboxdir)
	if err != nil {
		t.Fatal(err)
		return
	}
	if len(tb.Toolboxes) == 0 {
		t.Fatal("no tools found")
	}
	jso, err := tb.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	log.Printf("all: %s", string(jso))
	for _, toolbox := range tb.Toolboxes {
		for toolName, tool := range toolbox.Tools {
			log.Printf("tool %q: %s", toolName, js(tool))
		}
	}
}

func js(v json.Marshaler) string {
	b, err := v.MarshalJSON()
	if err != nil {
		panic(err)
	}
	return string(b)
}
