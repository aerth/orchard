package implv2

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

func ValidateFileSyntax(codeContent string, language *sitter.Language) error {
	parser := sitter.NewParser()
	parser.SetLanguage(language)

	ctx := context.Background()
	tree, err := parser.ParseCtx(ctx, nil, []byte(codeContent))
	if err != nil {
		return fmt.Errorf("tree-sitter compilation block failed: %w", err)
	}
	if tree == nil {
		return errors.New("tree-sitter returned a nil abstract syntax tree allocation")
	}
	defer tree.Close()

	rootNode := tree.RootNode()

	// Query pattern remains optimized for finding ERROR or MISSING tokens
	errorQuery, err := sitter.NewQuery([]byte(`[(ERROR) (MISSING)] @syntax_err`), language)
	if err != nil {
		return fmt.Errorf("failed to initialize syntax error validation query: %w", err)
	}
	defer errorQuery.Close()

	cursor := sitter.NewQueryCursor()
	cursor.Exec(errorQuery, rootNode)
	defer cursor.Close()
	var errMessages []string
	for match, ok := cursor.NextMatch(); ok; {
		for _, capture := range match.Captures {
			node := capture.Node
			startPoint := node.StartPoint()

			errLine := int(startPoint.Row) + 1
			errMessages = append(errMessages, fmt.Sprintf(
				"syntax error found near line %d (type: %s)",
				errLine, node.Type(),
			))
		}
	}

	if len(errMessages) > 0 {
		if len(errMessages) > 3 {
			errMessages = append(errMessages[:3], "...and more structural syntax issues")
		}
		return errors.New(strings.Join(errMessages, "; "))
	}

	return nil
}
