package schema2

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	yamlparser "github.com/goccy/go-yaml/parser"
	"github.com/xeipuuv/gojsonschema"
)

// -------------------------------------
// FILES - COLLECTIONS OF YAML DOCUMENTS
// -------------------------------------
func ValidateFile(file string) []ValidationError {
	lines := strings.FieldsFunc(file, func(r rune) bool { return r == '\n' })
	positions := GetDocumentPositions(file)
	var errors []ValidationError
	for _, docPos := range positions {
		contents := strings.Join(lines[docPos.Start:docPos.End], "\n")
		doc, ok := NewYamlDocument(contents)
		if !ok {
			errors = append(errors, ValidationError{
				Position: Position{
					LineStart: docPos.Start,
					LineEnd:   docPos.End,
					CharStart: 0,
					CharEnd:   0,
				},
				Message: "invalid yaml",
				Type:    "invalid_yaml",
			})
			continue
		}
		schema, found := GetSchema(contents)
		if !found {
			errors = append(errors, ValidationError{
				Position: Position{
					LineStart: docPos.Start,
					LineEnd:   docPos.End,
					CharStart: 0,
					CharEnd:   0,
				},
				Message: "No schema found",
				Type:    "no_schema_found",
			})
			continue
		}
		schemaErrors := schema.Validate(doc)
		if len(schemaErrors) == 0 {
			continue
		}
		paths := doc.Paths()
		for _, e := range schema.Validate(doc) {
			p, found := paths[e.Field]
			if !found {
				panic(fmt.Sprintf("expected path `%s` to exist in the document", e.Field))
			}
			errors = append(errors, ValidationError{
				Position: Position{
					LineStart: docPos.Start + p.LineStart,
					LineEnd:   docPos.Start + p.LineEnd,
					CharStart: p.CharStart,
					CharEnd:   p.CharEnd,
				},
				Message: e.Message,
				Type:    e.Type, // I've got life!
			})
		}
	}
	return errors
}

type ValidationError struct {
	Position Position
	Message  string
	Type     string
}
type Position struct{ LineStart, LineEnd, CharStart, CharEnd int } // [*Start, *End), 0-indexed

func GetDocumentPositions(file string) []LineRange {
	var ranges []LineRange
	startLine := 0
	lines := strings.FieldsFunc(file, func(r rune) bool { return r == '\n' })
	for i, line := range lines {
		if line == "---" {
			ranges = append(ranges, LineRange{
				Start: startLine,
				End:   i,
			})
			startLine = i + 1
		} else if i == len(lines)-1 {
			ranges = append(ranges, LineRange{
				Start: startLine,
				End:   i + 1,
			})
		}
	}
	return ranges
}

type LineRange struct{ Start, End int } // [Start, End), 0-indexed

// --------------
// YAML DOCUMENTS
// --------------

// yamlDocument is a valid yaml document
type yamlDocument string

// Returns "", false if the contents is not valid yaml
func NewYamlDocument(contents string) (yamlDocument, bool) {
	var throwaway map[string]any
	if err := yaml.Unmarshal([]byte(contents), &throwaway); err != nil {
		return "", false
	}
	return yamlDocument(contents), true
}

func (d yamlDocument) Paths() Paths {
	astFile, err := yamlparser.ParseBytes([]byte(d), 0)
	if err != nil {
		panic(fmt.Sprintf("expected a valid yaml document: %v", err))
	}
	if len(astFile.Docs) != 1 {
		panic(fmt.Sprintf("expected 1 document, got %d", len(astFile.Docs)))
	}
	paths := Paths{}
	ast.Walk(&paths, astFile.Docs[0])
	return paths
}

type Paths map[string]Position

var arrayPattern = regexp.MustCompile(`\[(\d+)\]`)

func (p Paths) Visit(node ast.Node) ast.Visitor {
	if node.Type() == ast.MappingValueType || node.Type() == ast.MappingType || node.Type() == ast.DocumentType {
		return p
	}
	path := strings.TrimPrefix(node.GetPath(), "$.")
	// Turn spec.ports[0].port into spec.ports.0.port
	path = arrayPattern.ReplaceAllString(path, ".$1")
	if _, found := p[path]; found {
		// Store the path to the key only, not the value
		// Assumes that the key is always visited first, couldn't find a way to distinguish
		// key nodes and value nodes
		return p
	}
	t := node.GetToken()
	p[path] = Position{
		LineStart: t.Position.Line - 1,
		LineEnd:   t.Position.Line,
		CharStart: t.Position.Column - 1,
		CharEnd:   t.Position.Column + len(t.Value) - 1,
	}
	return p
}

func (p Paths) AtCursor(line, char int) (string, bool) {
	for path, pos := range p {
		if pos.LineStart == line && pos.CharStart <= char && char < pos.CharEnd {
			return path, true
		}
	}
	return "", false
}

// ------------
// JSON SCHEMAS
// ------------

func GetSchema(contents string) (Schema, bool) {
	// TODO: Support other schemas than service-v1
	if strings.HasPrefix(contents, `kind: Service
apiVersion: v1`) {
		return Schema{
			loader: gojsonschema.NewReferenceLoader("https://raw.githubusercontent.com/yannh/kubernetes-json-schema/refs/heads/master/master-standalone-strict/service-v1.json"),
		}, true
	} else {
		return Schema{}, false
	}
}

type Schema struct{ loader gojsonschema.JSONLoader }

func (s *Schema) Fill() string { panic("todo") }
func (s *Schema) Docs() SchemaDocs {
	json, err := s.loader.LoadJSON()
	if err != nil {
		panic(fmt.Sprintf("expected schema to be valid json, got %v", err))
	}
	json_, ok := json.(map[string]any)
	if !ok {
		panic(fmt.Sprintf("expected schema to be a map[string]any, got %T", json))
	}
	docs := walkSchemaDocs("", json_)
	slices.SortFunc(docs, func(a, b Property) int {
		return strings.Compare(a.Path, b.Path)
	})
	return docs
}

type SchemaDocs []Property
type Property struct{ Path, Description, Type string }

// What should it do on anyOf, oneOf, allOf and not?
// - anyOf and oneOf: Return a list of SchemaDocs probably
// - allOf: Combine into one schema
// - not: Probably not support for docs
// schema identifiers:
// - type: string
// - type: array of strings
// - const
// - enum
// - x-kubernetes-preserve-unknown-fields
// - oneOf
// - anyOf

// Use ?<number> when there are multiple schemas to choose from as in anyOf and oneOf
//
// kind           The kind         string
// apiVersion     The apiVersion   string
// metadata       The metadata     object
// metadata.name  The name         string
// port?0         The port number  integer
// port?1         The port object  object
// port?1.number  The port number  integer
// port?1.name    The port name    string

func walkSchemaDocs(path string, schema map[string]any) SchemaDocs {
	var docs SchemaDocs
	type_, found := schema["type"]
	if found {
		var desc string
		if d, found := schema["description"]; found {
			desc = d.(string)
		}
		var resolvedType string
		tString, ok := type_.(string)
		if ok {
			resolvedType = tString
		} else {
			tArray, ok := type_.([]any)
			if ok {
				for _, it := range tArray {
					if it.(string) != "null" {
						resolvedType = it.(string)
						break
					}
				}
			} else {
				panic(fmt.Sprintf("expected the type to be a string or an array, got %v", type_))
			}
		}
		if path != "" {
			docs = append(docs, Property{
				Path:        path,
				Description: desc,
				Type:        resolvedType,
			})
		}
		switch resolvedType {
		case "object":
			properties_, found := schema["properties"]
			if !found {
				break
			}
			properties, ok := properties_.(map[string]any)
			if !ok {
				panic(fmt.Sprintf("expected properties to be map[string]any, got %T", properties_))
			}
			for property, subSchema_ := range properties {
				subSchema, ok := subSchema_.(map[string]any)
				if !ok {
					panic(fmt.Sprintf("expected schema to be map[string]any, got %T", subSchema_))
				}
				var subPath string
				if path == "" {
					subPath = property
				} else {
					subPath = path + "." + property
				}
				docs = append(docs, walkSchemaDocs(subPath, subSchema)...)
			}
		case "array":
			items_, found := schema["items"]
			if !found {
				panic("expected an array to have items")
			}
			items, ok := items_.(map[string]any)
			if !ok {
				panic(fmt.Sprintf("expected items to be map[string]any, got %T", items_))
			}
			var subPath string
			if path == "" {
				subPath = "[]"
			} else {
				subPath = path + "[]"
			}
			docs = append(docs, walkSchemaDocs(subPath, items)...)
		}
		return docs
	}

	var choices []any
	if oneOf_, found := schema["oneOf"]; found {
		oneOf, ok := oneOf_.([]any)
		if !ok {
			panic(fmt.Sprintf("expected oneOf to be []any, got %T", oneOf_))
		}
		choices = oneOf
	} else if anyOf_, found := schema["anyOf"]; found {
		anyOf, ok := anyOf_.([]any)
		if !ok {
			panic(fmt.Sprintf("expected anyOf to be []any, got %T", anyOf_))
		}
		choices = anyOf
	}
	if choices != nil {
		for i, choice_ := range choices {
			choice, ok := choice_.(map[string]any)
			if !ok {
				panic(fmt.Sprintf("expected an anyOf or oneOf element to be map[string]any, got %T", choice_))
			}
			docs = append(docs, walkSchemaDocs(fmt.Sprintf("%s?%d", path, i), choice)...)
		}
		return docs
	}
	panic("not supported")
}

type JsonValidationError struct{ Field, Message, Type string }

func (s *Schema) Validate(d yamlDocument) []JsonValidationError {
	jsonDocument, err := yaml.YAMLToJSON([]byte(d))
	if err != nil {
		panic(fmt.Sprintf("expected the yaml document to be convertable to json, got %v", err))
	}
	documentLoader := gojsonschema.NewBytesLoader(jsonDocument)
	res, err := gojsonschema.Validate(s.loader, documentLoader)
	if err != nil {
		panic(fmt.Sprintf("expected both schema and document to be valid, got %v", err))
	}
	var errors []JsonValidationError
	for _, e := range res.Errors() {
		field := e.Field()
		if e.Type() == "additional_property_not_allowed" {
			field = e.Field() + "." + e.Details()["property"].(string)
		}
		errors = append(errors, JsonValidationError{
			Field:   field,
			Message: e.Description(),
			Type:    e.Type(),
		})
	}
	return errors
}
