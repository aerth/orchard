package implv2

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

func ValidateFileSyntax(codeContent string, langname string, language *sitter.Language) error {
	slog.Info("validating syntax", "content", codeContent, "lang", langname)
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
	if !rootNode.HasError() { // 1.3.1, 1.3.2
		return nil
	}

	// Query pattern remains optimized for finding ERROR or MISSING tokens
	errorQuery, err := sitter.NewQuery([]byte(`
(ERROR) @syntax_err
_ @missing_err (#is-missing? @missing_err)
		`), language)
	if err != nil {
		return fmt.Errorf("failed to initialize syntax error validation query: %w", err)
	}
	defer errorQuery.Close()

	cursor := sitter.NewQueryCursor()
	cursor.Exec(errorQuery, rootNode)
	defer cursor.Close()
	var errMessages []string
	errMessages = domatches(cursor, []byte(codeContent))
	/*
		for match, ok := cursor.NextMatch(); ok; {
			slog.Info("match captures", "n", len(match.Captures))
			for _, capture := range match.Captures {
				if !capture.Node.HasError() {
					continue
				}
				slog.Info("got match", "cap", capture)
				node := capture.Node
				startPoint := node.StartPoint()

				errLine := int(startPoint.Row) + 1
				errMessages = append(errMessages, fmt.Sprintf(
					"syntax error found near line %d (type: %s)",
					errLine, node.Type(),
				))

			}
			if len(errMessages) > 0 {
				break
			}
		}
	*/
	if len(errMessages) > 0 {
		if len(errMessages) > 3 {
			errMessages = append(errMessages[:3], "...and more structural syntax issues")
		}
		return errors.New(strings.Join(errMessages, "; "))
	}

	return nil
}

func domatches(cursor *sitter.QueryCursor, codeBytes []byte) []string {
	var errMessages []string
	for match, ok := cursor.NextMatch(); ok; match, ok = cursor.NextMatch() {
		for _, capture := range match.Captures {
			targetNode := capture.Node

			// If the matched node is the global source_file wrapper,
			// hunt down the actual inner ERROR or MISSING leaf node child
			if targetNode.Type() == "source_file" {
				count := int(targetNode.ChildCount())
				for i := 0; i < count; i++ {
					child := targetNode.Child(i)
					if child != nil && (child.IsError() || child.IsMissing() || child.Type() == "ERROR") {
						targetNode = child
						break
					}
				}
			}

			startPoint := targetNode.StartPoint()
			errLine := int(startPoint.Row) + 1

			// Extract the exact broken code characters using byte boundary slicing
			startByte := targetNode.StartByte()
			endByte := targetNode.EndByte()

			var codeSnippet string
			if int(startByte) < len(codeBytes) && int(endByte) <= len(codeBytes) {
				codeSnippet = string(codeBytes[startByte:endByte])
				// Clean trailing newlines for readable log statements
				codeSnippet = strings.TrimSpace(codeSnippet)
			}

			errMessages = append(errMessages, fmt.Sprintf(
				"syntax error found near line %d (type: %s) fragment: %q",
				errLine, targetNode.Type(), codeSnippet,
			))
		}
	}
	return errMessages
}
