package parser

import (
	"fmt"
	"strings"

	"github.com/goccy/go-yaml/ast"
	yamlparser "github.com/goccy/go-yaml/parser"
)

type Position struct {
	Line, StartCol, EndCol int
}

type PathToPosition map[string]Position

func PathsToPositions(document []byte) (PathToPosition, error) {
	astFile, err := yamlparser.ParseBytes(document, 0)
	if err != nil {
		return nil, fmt.Errorf("parse document: %v", err)
	}
	if len(astFile.Docs) != 1 {
		return nil, fmt.Errorf("expected 1 document, got %d", len(astFile.Docs))
	}
	capturer := PathToPosition{}
	ast.Walk(&capturer, astFile.Docs[0])
	return capturer, nil
}

func (c PathToPosition) Visit(node ast.Node) ast.Visitor {
	if node.Type() == ast.MappingValueType || node.Type() == ast.MappingType || node.Type() == ast.DocumentType {
		return c
	}
	path := strings.TrimPrefix(node.GetPath(), "$.")
	if _, found := c[path]; found {
		// Store the path to the key only, not the value
		// Assumes that the key is always visited first, couldn't find a way to distinguish
		// key nodes and value nodes
		return c
	}
	t := node.GetToken()
	c[path] = Position{
		Line:     t.Position.Line,
		StartCol: t.Position.Column,
		EndCol:   t.Position.Column + len(t.Value),
	}
	return c
}

func PathAtPosition(document []byte, line, col int) (string, error) {
	paths, err := PathsToPositions(document)
	if err != nil {
		return "", fmt.Errorf("compute yaml paths to positions: %v", err)
	}
	for path, pos := range paths {
		if pos.Line == line && pos.StartCol <= col && col < pos.EndCol {
			return path, nil
		}
	}
	return "", nil
}
