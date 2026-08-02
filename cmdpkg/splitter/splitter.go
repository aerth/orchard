// splitter TODO: needs work
package splitter

import (
	"context"
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/python"
)

type Chunked struct {
	Name   string
	Source []byte
	*sitter.Tree
	splits []string // after processing
}

func Go(ctx context.Context, name string, source []byte) (Chunked, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(golang.GetLanguage())
	return Parse(ctx, parser, name, source)
}

func Py(ctx context.Context, name string, source []byte) (Chunked, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(python.GetLanguage())
	return Parse(ctx, parser, name, source)
}

func Parse(ctx context.Context, parser *sitter.Parser, name string, source []byte) (Chunked, error) {
	tree, err := parser.ParseCtx(ctx, nil, source)
	if err != nil {
		return Chunked{}, err
	}
	return Chunked{Name: name, Source: source, Tree: tree}, nil
}

// At 0 index
func (c *Chunked) At(n int, size int) string {
	if size == 0 {
		size = 1
	}
	if true {
		source := c.Source
		l := int(c.RootNode().ChildCount())
		if n >= l {
			return ""
		}
		start := c.RootNode().Child(n)
		end := start
		if n+size < l {
			end = c.RootNode().Child(n + size)
		}
		return fmt.Sprintf("Chunk %d+%d/%d\n%s", n, size-1, l, string(source[start.StartByte():end.EndByte()]))
		//	return c.RootNode().Content(input []byte)
	}
	node := c.RootNode()
	l := node.ChildCount()
	if n > int(l) {
		return ""
	}
	if c.splits == nil {
		c.splits = make([]string, l)
		for i := range int(l) {
			c.splits[i] = node.Child(i).Content(c.Source)
		}
	}
	end := min(n, int(l))
	return strings.Join(c.splits[n:end], "\n")
}
