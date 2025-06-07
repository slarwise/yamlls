package schema2

import (
	_ "embed"
	"testing"

	"github.com/xeipuuv/gojsonschema"
)

func TestValidateFile(t *testing.T) {

	tests := map[string]struct {
		file   string
		errors []ValidationError
	}{
		"valid": {
			file: `kind: Service
apiVersion: v1
metadata:
  name: hej
`,
			errors: nil,
		},
		"invalid": {
			file: `kind: Service
apiVersion: v1
metadata:
  name: hej
  asdf: wasd
`,
			errors: []ValidationError{
				{
					Position: Position{
						LineStart: 4,
						LineEnd:   5,
						CharStart: 2,
						CharEnd:   6,
					},
					Type: "additional_property_not_allowed",
				},
			},
		},
		"two-docs": {
			file: `kind: Service
apiVersion: v1
metadata:
  name: hej
---
kind: Service
apiVersion: v1
metadata:
  name: hej
  asdf: hej
`,
			errors: []ValidationError{
				{
					Position: Position{
						LineStart: 9,
						LineEnd:   10,
						CharStart: 2,
						CharEnd:   6,
					},
					Type: "additional_property_not_allowed",
				},
			},
		},
		"invalid-yaml": {
			file: `got punched for no: {}
reason
`,
			errors: []ValidationError{
				{
					Position: Position{
						LineStart: 0,
						LineEnd:   2,
						CharStart: 0,
						CharEnd:   0,
					},
					Type: "invalid_yaml",
				},
			},
		},
		"no-schema": {
			file: `kind: Server
apiVersion: 1990
`,
			errors: []ValidationError{
				{
					Position: Position{
						LineStart: 0,
						LineEnd:   2,
						CharStart: 0,
						CharEnd:   0,
					},
					Type: "no_schema_found",
				},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			errors := ValidateFile(test.file)
			if len(errors) != len(test.errors) {
				t.Fatalf("Expected %d errors, got %v", len(test.errors), errors)
			}
			for i, e := range errors {
				expected := test.errors[i]
				if e.Position != expected.Position {
					t.Fatalf("expected error at position `%v`, got `%v`", expected.Position, e.Position)
				}
				if e.Type != expected.Type {
					t.Fatalf("Expected type `%s`, got `%s`", expected.Type, e.Type)
				}
			}
		})
	}
}

func TestGetDocumentPositions(t *testing.T) {
	tests := map[string]struct {
		file   string
		ranges []LineRange
	}{
		"one-doc": {
			file: `hej: du
`,
			ranges: []LineRange{{0, 1}},
		},
		"one-doc-no-trailing-new-line": {
			file:   `hej: du`,
			ranges: []LineRange{{0, 1}},
		},
		"two-docs": {
			file: `hej: du
jag: heter
---
arvid: hej
what-if: the joker
was: blue
`,
			ranges: []LineRange{
				{0, 2},
				{3, 6},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ranges := GetDocumentPositions(test.file)
			if len(ranges) != len(test.ranges) {
				t.Fatalf("Expected %d ranges, got %d", test.ranges, ranges)
			}
			for i, r := range ranges {
				if r != test.ranges[i] {
					t.Fatalf("expected `%v`, got `%v`", test.ranges, r)
				}
			}
		})
	}
}

func TestSchemaValidate(t *testing.T) {
	tests := map[string]struct {
		schema string
		doc    yamlDocument
		errors []JsonValidationError
	}{
		"valid": {
			schema: `{"type": "object", "properties": {"name": {"type": "string"}}}`,
			doc:    "name: arvid",
			errors: nil,
		},
		"one-error": {
			schema: `{"type": "object", "properties": {"name": {"type": "string"}}}`,
			doc:    "name: 1",
			errors: []JsonValidationError{
				{
					Field: "name",
					Type:  "invalid_type",
				},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			s := Schema{gojsonschema.NewStringLoader(test.schema)}
			errors := s.Validate(test.doc)
			if len(errors) != len(test.errors) {
				t.Fatalf("Expected %d errors, got %v", len(test.errors), errors)
			}
			for i, e := range errors {
				expected := test.errors[i]
				if e.Field != expected.Field {
					t.Fatalf("expected error at field `%s`, got `%s`", expected.Field, e.Field)
				}
				if e.Type != expected.Type {
					t.Fatalf("Expected type `%s`, got `%s`", expected.Type, e.Type)
				}
			}
		})
	}
}

//go:embed testdata/oneOf.json
var oneOf string

//go:embed testdata/anyOf.json
var anyOf string

//go:embed testdata/const.json
var const_ string

//go:embed testdata/enum.json
var enum string

//go:embed testdata/x-kubernetes-preserve-unknown-fields.json
var xKubernetesPreserveUnknownFields string

//go:embed testdata/types.json
var types string

func TestSchemaDocs(t *testing.T) {
	tests := map[string]struct {
		schema string
		docs   SchemaDocs
	}{
		"simple": {
			schema: `{"type": "object", "properties": {"name": {"type": "string", "description": "The name of the person"}}}`,
			docs: SchemaDocs{
				{
					Path:        "name",
					Description: "The name of the person",
					Type:        "string",
				},
			},
		},
		"two-properties": {
			schema: `{"type": "object", "properties": {
					"name":    {"type": "string",  "description": "The name of the person"},
					"riddler": {"type": "boolean", "description": "riddle-riddle-riddle-riddle-riddle-diddle-diddle"}
				}}`,
			docs: SchemaDocs{
				{
					Path:        "name",
					Description: "The name of the person",
					Type:        "string",
				},
				{
					Path:        "riddler",
					Description: "riddle-riddle-riddle-riddle-riddle-diddle-diddle",
					Type:        "boolean",
				},
			},
		},
		"array": {
			schema: `{"type": "object", "properties": {
					"tonyz": {"type": "array", "description": "Tony Zarets", "items": {
						"type": "object",
						"description": "An epic gamer",
						"properties": {
							"producerIsHuman": {"type": "boolean", "description": "It is true"}
						}
					}}
				}}`,
			docs: SchemaDocs{
				{
					Path:        "tonyz",
					Description: "Tony Zarets",
					Type:        "array",
				},
				{
					Path:        "tonyz[]",
					Description: "An epic gamer",
					Type:        "object",
				},
				{
					Path:        "tonyz[].producerIsHuman",
					Description: "It is true",
					Type:        "boolean",
				},
			},
		},
		"oneOf": {
			schema: oneOf,
			docs: SchemaDocs{
				{
					Path:        "port",
					Description: "The port of the service",
					Type:        "oneOf",
				},
				{
					Path:        "port?0",
					Description: "The port name",
					Type:        "string",
				},
				{
					Path:        "port?1",
					Description: "The port number",
					Type:        "integer",
				},
			},
		},
		"anyOf": {
			schema: anyOf,
			docs: SchemaDocs{
				{
					Path:        "port",
					Description: "The port of the service",
					Type:        "anyOf",
				},
				{
					Path:        "port?0",
					Description: "The port name",
					Type:        "string",
				},
				{
					Path:        "port?1",
					Description: "The port number",
					Type:        "integer",
				},
			},
		},
		"const": {
			schema: const_,
			docs: SchemaDocs{
				{
					Path:        "kind",
					Description: "The service kind",
					Type:        "const",
				},
			},
		},
		"enum": {
			schema: enum,
			docs: SchemaDocs{
				{
					Path:        "level",
					Description: "The log level",
					Type:        "enum",
				},
			},
		},
		"x-kubernetes-preserve-unknown-fields": {
			schema: xKubernetesPreserveUnknownFields,
			docs: SchemaDocs{
				{
					Path:        "anything",
					Description: "An object that can be anything",
					Type:        "object",
				},
			},
		},
		"types": {
			schema: types,
			docs: SchemaDocs{
				{
					Path:        "port",
					Description: "The port of the service",
					Type:        "[string, integer]",
				},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			s := Schema{loader: gojsonschema.NewStringLoader(test.schema)}
			docs := s.Docs()
			t.Log(docs)
			if len(docs) != len(test.docs) {
				t.Fatalf("Expected %d properties with documentation, got %v", len(test.docs), docs)
			}
			for i, d := range docs {
				expected := test.docs[i]
				if d.Path != expected.Path {
					t.Fatalf("Expected path `%s`, got `%s`", expected.Path, d.Path)
				}
				if d.Description != expected.Description {
					t.Fatalf("Expected description `%s`, got `%s`", expected.Description, d.Description)
				}
				if d.Type != expected.Type {
					t.Fatalf("Expected type `%s`, got `%s`", expected.Type, d.Type)
				}
			}
		})
	}
}
