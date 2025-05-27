package parser

import (
	"fmt"
	"strings"

	"github.com/goccy/go-yaml"
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

// https://github.com/goccy/go-yaml/issues/574#issuecomment-2524814434
func ReplaceNode(document []byte, path string, replacement []byte) (string, error) {
	file, err := yamlparser.ParseBytes([]byte(document), yamlparser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("%+v", err)
	}
	node, err := yamlparser.ParseBytes([]byte(replacement), 0)
	if err != nil {
		return "", fmt.Errorf("%+v", err)
	}
	if len(node.Docs) == 0 {
		return "", fmt.Errorf("failed to parse replacement")
	}
	path_, err := yaml.PathString("$." + path)
	if err != nil {
		return "", fmt.Errorf("create path: %v", err)
	}
	if err := path_.ReplaceWithNode(file, node.Docs[0]); err != nil {
		return "", fmt.Errorf("%+v", err)
	}
	return file.String(), nil
}

type kindAndApiVersion struct {
	Kind       string `yaml:"kind"`
	ApiVersion string `yaml:"apiVersion"`
}

func GetKindAndApiVersion(document []byte) (string, string, error) {
	var result kindAndApiVersion
	if err := yaml.Unmarshal(document, &result); err != nil {
		return "", "", fmt.Errorf("invalid yaml: %v", err)
	}
	return result.Kind, result.ApiVersion, nil
}
