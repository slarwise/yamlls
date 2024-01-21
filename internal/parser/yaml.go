package parser

import (
	"errors"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/tidwall/gjson"
)

func GetPathAtPosition(line uint32, column uint32, text string) (string, error) {
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
	Lines        []uint32 // 0-indexed
	StartColumns []uint32 // 0-indexed
	EndColumns   []uint32
}

func (c *pathCapturer) Visit(node ast.Node) ast.Visitor {
	// if node.Type() != ast.StringType {
	// return c
	// }
	c.Paths = append(c.Paths, node.GetPath())
	token := node.GetToken()
	c.Lines = append(c.Lines, uint32(token.Position.Line-1))
	startColumn := token.Position.Column - 1
	c.StartColumns = append(c.StartColumns, uint32(startColumn))
	endColumn := startColumn + len(token.Value)
	c.EndColumns = append(c.EndColumns, uint32(endColumn))
	return c
}

func GetDescription(yamlPath string, schema []byte) (string, bool) {
	path := toSchemaPath(yamlPath)
	path = path + ".description"
	result := gjson.GetBytes(schema, path)
	if !result.Exists() {
		return "", false
	}
	return result.String(), true
}

func toSchemaPath(yamlPath string) string {
	schemaPath := strings.TrimPrefix(yamlPath, "$.")
	if schemaPath == "" {
		return ""
	}
	schemaPath = strings.ReplaceAll(schemaPath, ".", ".properties.")
	// Replace [\d+] with .items.
	regex := regexp.MustCompile(`\[\d+\]\.`)
	schemaPath = regex.ReplaceAllString(schemaPath, ".items.")
	return "properties." + schemaPath
}

// Completion
// - TODO: Enum values
// - Field properties
func GetProperties(yamlPath string, schema []byte) ([]string, bool) {
	schemaPath := toSchemaPath(yamlPath)
	propertiesPath := ""
	if schemaPath == "" {
		propertiesPath = "properties|@keys"
	} else {
		propertiesPath = schemaPath + ".properties|@keys"
	}
	result := gjson.GetBytes(schema, propertiesPath)
	if !result.Exists() {
		return nil, false
	}
	keys := []string{}
	for _, k := range result.Array() {
		keys = append(keys, k.Str)
	}
	return keys, true
}

func GetPathToParent(yamlPath string) string {
	if yamlPath == "$." {
		return "$."
	}
	nodes := strings.Split(yamlPath, ".")
	return strings.Join(nodes[:len(nodes)-1], ".")
}
