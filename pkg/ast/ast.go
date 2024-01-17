package ast

import (
	"errors"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// Lines and columns are 1-indexed
func GetPathAtPosition(line int, column int, text string) (string, error) {
	f, err := parser.ParseBytes([]byte(text), 0)
	if err != nil {
		return "", err
	}
	var capturer pathCapturer
	for _, doc := range f.Docs {
		ast.Walk(&capturer, doc.Body)
	}
	for i, p := range capturer.Paths {
		if line == capturer.Lines[i] && column >= capturer.StartColumns[i] && column < capturer.EndColumns[i] {
			return p, nil
		}
	}
	return "", errors.New("Not found")
}

type pathCapturer struct {
	Paths        []string
	Lines        []int // 1-indexed
	StartColumns []int // 1-indexed
	EndColumns   []int
}

func (c *pathCapturer) Visit(node ast.Node) ast.Visitor {
	if node.Type() != ast.StringType {
		return c
	}
	c.Paths = append(c.Paths, node.GetPath())
	token := node.GetToken()
	c.Lines = append(c.Lines, token.Position.Line)
	startColumn := token.Position.Column
	c.StartColumns = append(c.StartColumns, startColumn)
	endColumn := startColumn + len(token.Value)
	c.EndColumns = append(c.EndColumns, endColumn)
	return c
}
