package schema2

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	yamlparser "github.com/goccy/go-yaml/parser"
	"github.com/tidwall/gjson"
	"github.com/xeipuuv/gojsonschema"
)

func (s KubernetesStore) ValidateFile(file string) []ValidationError {
	lines := strings.FieldsFunc(file, func(r rune) bool { return r == '\n' })
	positions := getDocumentPositions(file)
	var errors []ValidationError
	for _, docPos := range positions {
		contents := strings.Join(lines[docPos.Start:docPos.End], "\n")
		doc, ok := newYamlDocument(contents)
		if !ok {
			errors = append(errors, ValidationError{
				Range: Range_{
					Start: Position{
						Line: docPos.Start,
						Char: 0,
					},
					End: Position{
						Line: docPos.End,
						Char: 0,
					},
				},
				Message: "invalid yaml",
				Type:    "invalid_yaml",
			})
			continue
		}
		schema, found := s.get(contents)
		if !found {
			continue
		}
		schemaErrors := schema.validate(doc)
		if len(schemaErrors) == 0 {
			continue
		}
		paths := doc.Paths()
		for _, e := range schema.validate(doc) {
			r, found := paths[e.Field]
			if !found {
				panic(fmt.Sprintf("expected path `%s` to exist in the document. Available paths: %v. Error type: %s", e.Field, paths, e.Type))
			}
			errors = append(errors, ValidationError{
				Range: Range_{
					Start: Position{
						Line: docPos.Start + r.Start.Line,
						Char: r.Start.Char,
					},
					End: Position{
						Line: docPos.Start + r.End.Line,
						Char: r.End.Char,
					},
				},
				Message: e.Message,
				Type:    e.Type, // I've got life!
			})
		}
	}
	return errors
}

type ValidationError struct {
	Range   Range_
	Message string
	Type    string
}

type Range_ struct{ Start, End Position } // zero-based, the start character is inclusive and the end character is exclusive
type Position struct{ Line, Char int }    // zero-based

func newRange(startLine, startChar, endLine, endChar int) Range_ {
	return Range_{
		Start: Position{Line: startLine, Char: startChar},
		End:   Position{Line: endLine, Char: endChar},
	}
}

func getDocumentPositions(file string) []lineRange {
	var ranges []lineRange
	startLine := 0
	lines := strings.FieldsFunc(file, func(r rune) bool { return r == '\n' })
	for i, line := range lines {
		if line == "---" {
			ranges = append(ranges, lineRange{
				Start: startLine,
				End:   i,
			})
			startLine = i + 1
		} else if i == len(lines)-1 {
			ranges = append(ranges, lineRange{
				Start: startLine,
				End:   i + 1,
			})
		}
	}
	return ranges
}

type lineRange struct{ Start, End int } // [Start, End), 0-indexed

// yamlDocument is a valid yaml document
type yamlDocument string

// Returns "", false if the contents is not valid yaml
func newYamlDocument(contents string) (yamlDocument, bool) {
	var throwaway map[string]any
	if err := yaml.Unmarshal([]byte(contents), &throwaway); err != nil {
		return "", false
	}
	return yamlDocument(contents), true
}

func (d yamlDocument) Paths() paths {
	astFile, err := yamlparser.ParseBytes([]byte(d), 0)
	if err != nil {
		panic(fmt.Sprintf("expected a valid yaml document: %v", err))
	}
	if len(astFile.Docs) != 1 {
		panic(fmt.Sprintf("expected 1 document, got %d", len(astFile.Docs)))
	}
	paths := paths{}
	ast.Walk(&paths, astFile.Docs[0])
	return paths
}

type paths map[string]Range_

var (
	arrayPattern          = regexp.MustCompile(`\[(\d+)\]`)
	endingIndex           = regexp.MustCompile(`\.(\d+)$`)
	endingIndexInBrackets = regexp.MustCompile(`\[(\d+)\]$`)
)

func (p paths) Visit(node ast.Node) ast.Visitor {
	if node.Type() == ast.MappingValueType || node.Type() == ast.DocumentType {
		return p
	}
	path := strings.TrimPrefix(node.GetPath(), "$.")
	if path == "$" {
		return p
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
		p[path] = Range_{
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
	// path = arrayPattern.ReplaceAllString(path, ".$1")
	if _, found := p[path]; found {
		// Store the path to the key only, not the value
		// Assumes that the key is always visited first, couldn't find a way to distinguish
		// key nodes and value nodes
		return p
	}
	t := node.GetToken()
	p[path] = Range_{
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

func (p paths) AtCursor(line, char int) (string, bool) {
	for path, r := range p {
		if r.Start.Line == line && r.Start.Char <= char && char < r.End.Char {
			return path, true
		}
	}
	return "", false
}

type schema struct{ loader gojsonschema.JSONLoader }

func (s *schema) Fill() string { panic("todo") }
func (s *schema) Docs() []SchemaProperty {
	loadedSchema_, err := s.loader.LoadJSON()
	if err != nil {
		panic(fmt.Sprintf("expected schema to be valid json, got %v", err))
	}
	loadedSchema, ok := loadedSchema_.(map[string]any)
	if !ok {
		panic(fmt.Sprintf("expected schema to be a map[string]any, got %T", loadedSchema_))
	}
	bytes, err := json.Marshal(loadedSchema_)
	if err != nil {
		panicf("marshal schema back to json: %v", err)
	}
	docs := walkSchemaDocs("", loadedSchema, bytes)
	slices.SortFunc(docs, func(a, b SchemaProperty) int {
		return strings.Compare(a.Path, b.Path)
	})
	return docs
}

// Send in an empty string for highlightProperty to not go to
// the property when opening it in a browser
func (s *schema) HtmlDocs(highlightProperty string) string {
	docs := s.Docs()
	output := strings.Builder{}
	fmt.Fprint(&output, `<!DOCTYPE html>
<html>
<head>
  <title>Documentation</title>
  <style>
    body {background-color: #3f3f3f; color: #DCDCCC; font-size: 18px; }
    code {font-size: 80%;}
    code.required {color: #E0CF9F;}
    span.path {color: #DCA3A3; }
  </style>
</head>
`)
	fmt.Fprintln(&output, "<body>")

	for _, property := range docs {
		fmt.Fprintln(&output, "  <p>")

		requiredClass := ""
		if property.Required {
			requiredClass = ` class="required"`
		}
		fmt.Fprintf(&output, `    <span class="path" id="%s">%s</span> <code%s>[%s]</code>`, property.Path, property.Path, requiredClass, property.Type)

		fmt.Fprintln(&output)
		if property.Description != "" {
			fmt.Fprint(&output, "    <br>\n")
			fmt.Fprintf(&output, "    %s\n", property.Description)
		}
		fmt.Fprintln(&output, "  </p>")
	}

	if highlightProperty != "" {
		fmt.Fprintf(&output, `  <script>window.location.hash = "%s"</script>`, highlightProperty)
		fmt.Fprintln(&output, "")
	}

	fmt.Fprintln(&output, "</body>")
	fmt.Fprintln(&output, "</html>")
	return output.String()
}

type SchemaProperty struct {
	Path, Description, Type string
	Required                bool
}

// What should it do on anyOf, oneOf, allOf and not?
// - anyOf and oneOf: Return a list of SchemaDocs probably
// - allOf: Combine into one schema
// - not: Probably not support for docs
// schema identifiers:
// - [x] type: string
// - [ ] type: array of strings
// - [x] const
// - [x] enum
// - [x] x-kubernetes-preserve-unknown-fields
// - [x] oneOf
// - [x] anyOf
// - [ ] allOf

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

// TODO: Maybe the root should be `.` instead of any empty string

var rootChoicePattern = regexp.MustCompile(`^\?\d+$`)

func walkSchemaDocs(path string, schema map[string]any, rootSchema []byte) []SchemaProperty {
	var docs []SchemaProperty
	var desc string
	if d, found := schema["description"]; found {
		desc = d.(string)
	}

	schemaTypes := schemaType(schema)
	var typeString string
	switch len(schemaTypes) {
	case 0:
		panic("schemaType() returned an empty list, this shouldn't happen")
	case 1:
		switch schemaTypes[0] {
		case "object":
			properties_, found := schema["properties"]
			if !found {
				break
			}
			properties, ok := properties_.(map[string]any)
			if !ok {
				panicf("expected properties to be map[string]any, got %T", properties_)
			}
			var requiredProperties []string
			if required_, found := schema["required"]; found {
				required, ok := required_.([]any)
				if ok {
					for _, p := range required {
						requiredProperties = append(requiredProperties, p.(string))
					}
				}
			}
			for property, subSchema_ := range properties {
				subSchema, ok := subSchema_.(map[string]any)
				if !ok {
					panicf("expected schema to be map[string]any, got %T", subSchema_)
				}
				subPath := property
				if path != "" {
					subPath = path + "." + property
				}
				subDocs := walkSchemaDocs(subPath, subSchema, rootSchema)
				if slices.Contains(requiredProperties, property) {
					subDocs[0].Required = true
				}
				docs = append(docs, subDocs...)
			}
			typeString = schemaTypes[0]
		case "array":
			items_, found := schema["items"]
			if !found {
				panic("expected an array to have items")
			}
			items, ok := items_.(map[string]any)
			if !ok {
				panicf("expected items to be map[string]any, got %T", items_)
			}
			subPath := "[]"
			if path != "" {
				subPath = path + "[]"
			}
			docs = append(docs, walkSchemaDocs(subPath, items, rootSchema)...)
			typeString = schemaTypes[0]
		case "oneOf", "anyOf":
			typeString = schemaTypes[0]
			if choices_, found := schema[typeString]; found {
				choices, ok := choices_.([]any)
				if !ok {
					panicf("expected %s to be []any, got %T", typeString, choices)
				}
				for i, choice_ := range choices {
					choice, ok := choice_.(map[string]any)
					if !ok {
						panicf("expected an %s element to be map[string], got %T", typeString, choice_)
					}
					docs = append(docs, walkSchemaDocs(fmt.Sprintf("%s?%d", path, i), choice, rootSchema)...)
				}
			}
		case "allOf":
			typeString = schemaTypes[0]
			elements_ := schema[typeString]
			elements, ok := elements_.([]any)
			if !ok {
				panicf("expected allOf to be []any, got %T", elements_)
			}
			for _, element_ := range elements {
				element, ok := element_.(map[string]any)
				if !ok {
					panicf("expected an allOf element to be map[string]any, got %T", element_)
				}
				docs = append(docs, walkSchemaDocs(path, element, rootSchema)...)
			}
		case "$ref":
			if _, found := schema["$ref"]; !found {
				panicf("expected type $ref to have property $ref, got %+v", schema)
			}
			ref, ok := schema["$ref"].(string)
			if !ok {
				panicf("expected $ref to be a string, got %v", schema["$ref"])
			}
			// NOTE: We expect all references to be part of the same file
			ref = strings.Split(ref, "#")[1]
			refPath := strings.ReplaceAll(ref[1:], "/", ".")
			res := gjson.GetBytes(rootSchema, refPath)
			if !res.Exists() {
				panicf("could not find the reference at path %s in the root schema %s", refPath, rootSchema)
			}
			refSchema, ok := res.Value().(map[string]any)
			if !ok {
				panicf("expected ref to point to an object")
			}
			docs = append(docs, walkSchemaDocs(path, refSchema, rootSchema)...)
			return docs
		case "x-kubernetes-preserve-unknown-fields":
			typeString = "object"
		default:
			typeString = schemaTypes[0]
		}
	default:
		typeString = "[" + strings.Join(schemaTypes, ", ") + "]"
		if slices.Contains(schemaTypes, "object") {
			// TODO: Duplicate code with type == "object" above
			properties_, found := schema["properties"]
			if !found {
				break
			}
			properties, ok := properties_.(map[string]any)
			if !ok {
				panicf("expected properties to be map[string]any, got %T", properties_)
			}
			var requiredProperties []string
			if required_, found := schema["required"]; found {
				required, ok := required_.([]any)
				if ok {
					for _, p := range required {
						requiredProperties = append(requiredProperties, p.(string))
					}
				}
			}
			for property, subSchema_ := range properties {
				subSchema, ok := subSchema_.(map[string]any)
				if !ok {
					panicf("expected schema to be map[string]any, got %T", subSchema_)
				}
				subPath := property
				if path != "" {
					subPath = path + "." + property
				}
				subDocs := walkSchemaDocs(subPath, subSchema, rootSchema)
				if slices.Contains(requiredProperties, property) {
					subDocs[0].Required = true
				}
				docs = append(docs, subDocs...)
			}
		} else if slices.Contains(schemaTypes, "array") {
			panicf("multiple types containing `array` is not supported, got %v", schemaTypes)
		}
	}
	if path != "" && !rootChoicePattern.MatchString(path) {
		docs = append(docs, SchemaProperty{
			Path:        path,
			Description: desc,
			Type:        typeString,
		})
	}
	return docs
}

func panicf(format string, args ...any) {
	panic(fmt.Sprintf(format, args...))
}

// The return value will have at least one element
func schemaType(schema map[string]any) []string {
	// Prioritize anyOf, oneOf, allOf over type: object
	if _, found := schema["anyOf"]; found {
		return []string{"anyOf"}
	} else if _, found := schema["oneOf"]; found {
		return []string{"oneOf"}
	} else if _, found := schema["allOf"]; found {
		return []string{"allOf"}
	} else if _, found := schema["const"]; found {
		return []string{"const"}
	} else if _, found := schema["enum"]; found {
		return []string{"enum"}
	} else if _, found := schema["x-kubernetes-preserve-unknown-fields"]; found {
		return []string{"x-kubernetes-preserve-unknown-fields"}
	} else if _, found := schema["$ref"]; found {
		return []string{"$ref"}
	} else if type_, found := schema["type"]; found {
		switch type_ := type_.(type) {
		case string:
			return []string{type_}
		case []any:
			var types []string
			for _, t := range type_ {
				if t, ok := t.(string); ok {
					types = append(types, t)
				} else {
					panicf("expected type all elements in `type` to be strings, got %v", t)
				}
			}
			return types
		default:
			panicf("expected type to be a string or an array, got %v", type_)
		}
	}
	panic(fmt.Sprintf("could not figure out the type of this schema: %v", schema))
}

type jsonValidationError struct{ Field, Message, Type string }

func (s *schema) validate(d yamlDocument) []jsonValidationError {
	jsonDocument, err := yaml.YAMLToJSON([]byte(d))
	if err != nil {
		panic(fmt.Sprintf("expected the yaml document to be convertable to json, got %v", err))
	}
	documentLoader := gojsonschema.NewBytesLoader(jsonDocument)
	res, err := gojsonschema.Validate(s.loader, documentLoader)
	if err != nil {
		panic(fmt.Sprintf("expected both schema and document to be valid, got %v", err))
	}
	var errors []jsonValidationError
	for _, e := range res.Errors() {
		field := e.Field()
		if e.Type() == "additional_property_not_allowed" {
			field = e.Field() + "." + e.Details()["property"].(string)
		}
		errors = append(errors, jsonValidationError{
			Field:   field,
			Message: e.Description(),
			Type:    e.Type(),
		})
	}
	return errors
}

var arrayPath = regexp.MustCompile(`\.\d+`)

// Documentation in html format, with the focus placed on line and char.
// Does anyone want another format?
func (s KubernetesStore) HtmlDocumentation(file string, line int, char int) (string, bool) {
	ranges := getDocumentPositions(file)
	var maybeValidDocument string
	for _, r := range ranges {
		if r.Start <= line && line < r.End {
			lines := strings.FieldsFunc(file, func(r rune) bool { return r == '\n' })
			maybeValidDocument = strings.Join(lines[r.Start:r.End], "\n")
			line = line - r.Start
		}
	}
	if maybeValidDocument == "" {
		return "", false
	}
	var pathAtCursor string
	document, valid := newYamlDocument(maybeValidDocument)
	if valid {
		paths := document.Paths()
		var found bool
		pathAtCursor, found = paths.AtCursor(line, char)
		if found {
			// Turn spec.ports.0.name into spec.ports[].name
			pathAtCursor = arrayPath.ReplaceAllString(pathAtCursor, "[]")
		}
	}
	schema, schemaFound := s.get(string(document))
	if !schemaFound {
		return "", false
	}
	return schema.HtmlDocs(pathAtCursor), true
}

type Error error

var (
	ErrSchemaNotFound         Error = errors.New("schema not found")
	ErrPathNotFound           Error = errors.New("path not found")
	ErrInvalidDocument        Error = errors.New("invalid document")
	ErrDocumentNotFound       Error = errors.New("document not found")
	ErrNoDocumentationForPath Error = errors.New("no documentation for path")
)

func (s KubernetesStore) DocumentationAtCursor(file string, line, char int) (SchemaProperty, Error) {
	ranges := getDocumentPositions(file)
	var maybeValidDocument string
	for _, r := range ranges {
		if r.Start <= line && line < r.End {
			lines := strings.FieldsFunc(file, func(r rune) bool { return r == '\n' })
			maybeValidDocument = strings.Join(lines[r.Start:r.End], "\n")
			line = line - r.Start
		}
	}
	if maybeValidDocument == "" {
		return SchemaProperty{}, ErrDocumentNotFound
	}
	document, valid := newYamlDocument(maybeValidDocument)
	if !valid {
		return SchemaProperty{}, ErrInvalidDocument
	}
	paths := document.Paths()
	path, found := paths.AtCursor(line, char)
	if !found {
		// Happens if the cursor is not on a field or on an empty space
		return SchemaProperty{}, ErrPathNotFound
	}
	schema, schemaFound := s.get(string(document))
	if !schemaFound {
		return SchemaProperty{}, ErrSchemaNotFound
	}
	// Turn spec.ports.0.name into spec.ports[].name
	path = arrayPath.ReplaceAllString(path, "[]")
	pathFound := false
	properties := schema.Docs()
	var property SchemaProperty
	for _, p := range properties {
		if p.Path == path {
			property = p
			pathFound = true
			break
		}
	}
	if !pathFound {
		return SchemaProperty{}, ErrNoDocumentationForPath
	}
	return property, nil
}
