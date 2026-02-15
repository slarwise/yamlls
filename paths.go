package main

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/goccy/go-yaml/ast"
	yamlparser "github.com/goccy/go-yaml/parser"
)

type Range struct{ Start, End Position } // zero-based, the start character is inclusive and the end character is exclusive
type Position struct{ Line, Char int }   // zero-based

func newRange(startLine, startChar, endLine, endChar int) Range {
	return Range{
		Start: Position{Line: startLine, Char: startChar},
		End:   Position{Line: endLine, Char: endChar},
	}
}

func documentPaths(doc string) Paths {
	astFile, err := yamlparser.ParseBytes([]byte(doc), 0)
	if err != nil {
		panicf("expected a valid yaml document: %v", err)
	}
	if len(astFile.Docs) != 1 {
		panicf("expected 1 document, got %d", len(astFile.Docs))
	}
	paths := Paths{}
	ast.Walk(&paths, astFile.Docs[0])
	return paths
}

type Paths map[string]Range

var (
	arrayPattern = regexp.MustCompile(`\[(\d+)\]`)
	endingIndex  = regexp.MustCompile(`\.(\d+)$`)
)

func (p Paths) Visit(node ast.Node) ast.Visitor {
	if node.Type() == ast.MappingValueType || node.Type() == ast.DocumentType {
		return p
	}
	path := strings.TrimPrefix(node.GetPath(), "$")
	if path == "" {
		path = "."
	}
	path = arrayPattern.ReplaceAllString(path, ".$1")
	if node.Type() == ast.MappingType && endingIndex.MatchString(path) {
		// The path looks like spec.ports[1] here
		// This is the `:` in the first element in an object array
		// Not sure why it's only on the first one
		// Use the parent path to compute the column
		parent := endingIndex.ReplaceAllString(path, "")
		var char int
		for existingPath, pos := range p {
			if existingPath == parent {
				char = pos.Start.Char + 2 // NOTE: Assuming that lists are indented here
			}
		}
		t := node.GetToken()
		p[path] = Range{
			Start: Position{
				Line: t.Position.Line - 1,
				Char: char,
			},
			End: Position{
				Line: t.Position.Line - 1,
				Char: char + 1,
			},
		}
		return p
	}
	// Turn spec.ports[0].port into spec.ports.0.port
	if _, found := p[path]; found {
		// Store the path to the key only, not the value
		// Assumes that the key is always visited first, couldn't find a way to distinguish
		// key nodes and value nodes
		return p
	}
	t := node.GetToken()
	if path == "." {
		p[path] = Range{}
		return p
	}
	p[path] = Range{
		Start: Position{
			Line: t.Position.Line - 1,
			Char: t.Position.Column - 1,
		},
		End: Position{
			Line: t.Position.Line - 1,
			Char: t.Position.Column + len(t.Value) - 1,
		},
	}
	return p
}

func pathAtPosition(doc string, line, char int) (string, Range, bool) {
	for path, range_ := range documentPaths(doc) {
		if range_.Start.Line == line && range_.Start.Char <= char && char < range_.End.Char {
			return path, range_, true
		}
	}
	return "", Range{}, false
}

func pathToSchemaPath(path string) string {
	schemaSegments := []string{}
	segments := strings.FieldsFunc(path, func(r rune) bool { return r == '.' })
	for _, segment := range segments {
		isArrayIndex := true
		for _, r := range segment {
			if !unicode.IsDigit(r) {
				isArrayIndex = false
				break
			}
		}
		if isArrayIndex {
			schemaSegments = append(schemaSegments, "items")
		} else {
			schemaSegments = append(schemaSegments, "properties", segment)
		}
	}
	return strings.Join(schemaSegments, ".")
}
