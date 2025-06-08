package schema2

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	yamlparser "github.com/goccy/go-yaml/parser"
	"github.com/xeipuuv/gojsonschema"
)

func ValidateFile(file string, store Store) []validationError {
	lines := strings.FieldsFunc(file, func(r rune) bool { return r == '\n' })
	positions := getDocumentPositions(file)
	var errors []validationError
	for _, docPos := range positions {
		contents := strings.Join(lines[docPos.Start:docPos.End], "\n")
		doc, ok := newYamlDocument(contents)
		if !ok {
			errors = append(errors, validationError{
				Range: range_{
					Start: position{
						Line: docPos.Start,
						Char: 0,
					},
					End: position{
						Line: docPos.End,
						Char: 0,
					},
				},
				Message: "invalid yaml",
				Type:    "invalid_yaml",
			})
			continue
		}
		schema, found := store.get(contents)
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
			errors = append(errors, validationError{
				Range: range_{
					Start: position{
						Line: docPos.Start + r.Start.Line,
						Char: r.Start.Char,
					},
					End: position{
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

type validationError struct {
	Range   range_
	Message string
	Type    string
}

type range_ struct{ Start, End position } // zero-based, the start character is inclusive and the end character is exclusive
type position struct{ Line, Char int }    // zero-based

func newRange(startLine, startChar, endLine, endChar int) range_ {
	return range_{
		Start: position{Line: startLine, Char: startChar},
		End:   position{Line: endLine, Char: endChar},
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

type paths map[string]range_

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
		p[path] = range_{
			Start: position{
				Line: t.Position.Line - 1,
				Char: char,
			},
			End: position{
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
	p[path] = range_{
		Start: position{
			Line: t.Position.Line - 1,
			Char: t.Position.Column - 1,
		},
		End: position{
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
func (s *schema) Docs() schemaProperties {
	json, err := s.loader.LoadJSON()
	if err != nil {
		panic(fmt.Sprintf("expected schema to be valid json, got %v", err))
	}
	json_, ok := json.(map[string]any)
	if !ok {
		panic(fmt.Sprintf("expected schema to be a map[string]any, got %T", json))
	}
	docs := walkSchemaDocs("", json_)
	slices.SortFunc(docs, func(a, b schemaProperty) int {
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

type schemaProperties []schemaProperty
type schemaProperty struct {
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

// TODO: Support $ref keyword
func walkSchemaDocs(path string, schema map[string]any) schemaProperties {
	var docs schemaProperties
	var desc string
	if d, found := schema["description"]; found {
		desc = d.(string)
	}

	type_, found := schema["type"]
	if found {
		var types []string
		switch type_ := type_.(type) {
		case string:
			types = []string{type_}
		case []any:
			for _, t_ := range type_ {
				t, ok := t_.(string)
				if !ok {
					panic(fmt.Sprintf("expected all elements in `type` to be a string, got %v", t_))
				}
				if t != "null" {
					types = append(types, t)
				}
			}
		default:
			panic(fmt.Sprintf("expected type to be a string or an array, got %v", type_))
		}
		typeString := types[0]
		if len(types) > 1 {
			typeString = fmt.Sprintf("[%s]", strings.Join(types, ", "))
		}
		if path != "" {
			docs = append(docs, schemaProperty{
				Path:        path,
				Description: desc,
				Type:        typeString,
			})
		}
		if len(types) == 1 {
			switch types[0] {
			case "object":
				properties_, found := schema["properties"]
				if !found {
					break
				}
				properties, ok := properties_.(map[string]any)
				if !ok {
					panic(fmt.Sprintf("expected properties to be map[string]any, got %T", properties_))
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
						panic(fmt.Sprintf("expected schema to be map[string]any, got %T", subSchema_))
					}
					var subPath string
					if path == "" {
						subPath = property
					} else {
						subPath = path + "." + property
					}
					subDocs := walkSchemaDocs(subPath, subSchema)
					if slices.Contains(requiredProperties, property) {
						subDocs[0].Required = true
					}
					docs = append(docs, subDocs...)
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
		} else {
			if slices.Contains(types, "object") || slices.Contains(types, "array") {
				panic("multiple types containing object or array not supported")
			}
		}
		return docs
	}

	for _, choiceType := range []string{"oneOf", "anyOf"} {
		if choices_, found := schema[choiceType]; found {
			choices, ok := choices_.([]any)
			if !ok {
				panic(fmt.Sprintf("expected oneOf to be []any, got %T", choices_))
			}
			if path != "" {
				docs = append(docs, schemaProperty{
					Path:        path,
					Description: desc,
					Type:        choiceType,
				})
			}
			for i, choice_ := range choices {
				choice, ok := choice_.(map[string]any)
				if !ok {
					panic(fmt.Sprintf("expected an anyOf or oneOf element to be map[string]any, got %T", choice_))
				}
				docs = append(docs, walkSchemaDocs(fmt.Sprintf("%s?%d", path, i), choice)...)
			}
			return docs
		}
	}

	if _, found := schema["const"]; found {
		if path != "" {
			docs = append(docs, schemaProperty{
				Path:        path,
				Description: desc,
				Type:        "const",
			})
		}
		return docs
	}

	if _, found := schema["enum"]; found {
		if path != "" {
			docs = append(docs, schemaProperty{
				Path:        path,
				Description: desc,
				Type:        "enum",
			})
		}
		return docs
	}

	if _, found := schema["x-kubernetes-preserve-unknown-fields"]; found {
		if path != "" {
			docs = append(docs, schemaProperty{
				Path:        path,
				Description: desc,
				Type:        "object",
			})
		}
		return docs
	}

	panic(fmt.Sprintf("schema not supported %v", schema))
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

// Documentation in html format, with the focus placed on line and char
// Does anyone want another format?
func HtmlDocumentation(file string, line int, char int, store Store) (string, bool) {
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
	schema, schemaFound := store.get(string(document))
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

func DocumentationAtCursor(file string, line, char int, store Store) (schemaProperty, Error) {
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
		return schemaProperty{}, ErrDocumentNotFound
	}
	document, valid := newYamlDocument(maybeValidDocument)
	if !valid {
		return schemaProperty{}, ErrInvalidDocument
	}
	paths := document.Paths()
	path, found := paths.AtCursor(line, char)
	if !found {
		// Happens if the cursor is not on a field or on an empty space
		return schemaProperty{}, ErrPathNotFound
	}
	schema, schemaFound := store.get(string(document))
	if !schemaFound {
		return schemaProperty{}, ErrSchemaNotFound
	}
	// Turn spec.ports.0.name into spec.ports[].name
	path = arrayPath.ReplaceAllString(path, "[]")
	pathFound := false
	properties := schema.Docs()
	var property schemaProperty
	for _, p := range properties {
		if p.Path == path {
			property = p
			pathFound = true
			break
		}
	}
	if !pathFound {
		return schemaProperty{}, ErrNoDocumentationForPath
	}
	return property, nil
}
