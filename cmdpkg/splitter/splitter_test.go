package splitter

import (
	"embed"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
)

//go:embed *.go
var embedded embed.FS

func TestSplit(t *testing.T) {
	src, err := embedded.ReadFile("splitter.go")
	if err != nil {
		panic(err.Error())
	}
	gots, err := Go(t.Context(), "splitter.go", src)
	if err != nil {
		t.Fatal(err.Error())
	}
	log.Printf("count: %d", gots.RootNode().ChildCount())

	const size = 3
	for i := range int(gots.RootNode().ChildCount()) / size {
		got := gots.At(i*size, size)
		_ = got
		labels := Types(gots.RootNode().Child(i * size))

		for i := range size {
			labels = append(labels, Types(gots.RootNode().Child((i*size)+size))...)
		}
		fmt.Fprintf(os.Stderr, "\nWow %d (%s)\n\n```go\n%s\n```\n", i, strings.Join(labels, ","), got)
	}
}

// TODO: improve
func Types(node *sitter.Node) []string {
	x := make([]string, int(node.ChildCount()))
	for i := range x {
		label := node.Child(i).Type()
		x[i] = label
	}
	return x
}
