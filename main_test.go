package main

import (
	_ "embed"
	"encoding/json"
	"testing"
)

func TestValidateFile(t *testing.T) {
	tests := map[string]struct {
		contents string
		errors   []ValidationError
	}{
		"one-document/valid": {
			contents: `kind: Ingress
apiVersion: networking.k8s.io/v1
metadata:
  name: myingress
spec:
  rules:
    - host: example.com
      http:
        paths:
          - path: /
            pathType: ImplementationSpecific
            backend:
              service:
                name: myapp
`,
			errors: nil,
		},
		"one-document/invalid-yaml": {
			contents: `kind: Ingress
apiVersion: networking.k8s.io/v1
spec: [
`,
			errors: []ValidationError{
				{
					Range:    newRange(0, 0, 3, 0),
					Type:     "invalid_yaml",
					Severity: SEVERITY_ERROR,
				},
			},
		},
		"one-document/no-schema-found": {
			contents: `kind: Ingress
apiVersion: does-not-exist
`,
			errors: []ValidationError{
				{
					Range:    newRange(0, 0, 0, 0),
					Type:     "no_schema_found",
					Severity: SEVERITY_WARN,
				},
			},
		},
		"two-documents/valid": {
			contents: `kind: Ingress
apiVersion: networking.k8s.io/v1
metadata:
  name: myingress
spec:
  rules:
    - host: example.com
      http:
        paths:
          - path: /
            pathType: ImplementationSpecific
            backend:
              service:
                name: myapp
---
kind: Service
apiVersion: v1
metadata:
  name: myapp
spec:
  ports:
    - port: 8080
      name: http
`,
			errors: nil,
		},
		"two-documents/invalid-yaml": {
			contents: `kind: Ingress
apiVersion: networking.k8s.io/v1
metadata:
  name: myingress
spec:
  rules:
    - host: example.com
      http:
        paths:
          - path: /
            pathType: ImplementationSpecific
            backend:
              service:
                name: myapp
---
kind: [
apiVersion: v1
metadata:
  name: myapp
spec:
  ports:
    - port: 8080
      name: http
`,
			errors: []ValidationError{
				{
					Range:    newRange(15, 0, 23, 0),
					Type:     "invalid_yaml",
					Severity: SEVERITY_ERROR,
				},
			},
		},
		"two-document/no-schema-found": {
			contents: `kind: Ingress
apiVersion: does-not-exist
---
kind: Service
apiVersion: v1
metadata:
  name: myapp
spec:
  ports:
    - port: 8080
      name: http
`,
			errors: []ValidationError{
				{
					Range:    newRange(0, 0, 0, 0),
					Type:     "no_schema_found",
					Severity: SEVERITY_WARN,
				},
			},
		},
		"one-document/no-kind-and-apiVersion": {
			contents: `hej: du
`,
			errors: nil,
		},
		"one-document/additional-property": {
			contents: `kind: Ingress
apiVersion: networking.k8s.io/v1
hej: du
`,
			errors: []ValidationError{
				{
					Range:    newRange(2, 0, 2, 3),
					Type:     "additional_property_not_allowed",
					Severity: SEVERITY_ERROR,
				},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			errors, fail := fileValidate(test.contents)
			if fail != VALIDATION_FAILURE_REASON_NOT_A_FAILURE {
				t.Fatalf("expected validation to work, got %s", fail)
			}
			if len(errors) != len(test.errors) {
				t.Fatalf("expected %d errors, got %v", len(test.errors), errors)
			}
			for i := range errors {
				expectedError := test.errors[i]
				if errors[i].Type != expectedError.Type {
					t.Fatalf("expected type `%s`, got `%s`", expectedError.Type, errors[i].Type)
				}
				if errors[i].Range != expectedError.Range {
					t.Fatalf("expected range %v, got %v", expectedError.Range, errors[i].Range)
				}
				if errors[i].Severity != expectedError.Severity {
					t.Fatalf("expected severity %v, got %v", expectedError.Severity, errors[i].Severity)
				}
			}
		})
	}
}

func TestGetDocumentPositions(t *testing.T) {
	tests := map[string]struct {
		file   string
		ranges []lineRange
	}{
		"one-doc": {
			file: `hej: du
`,
			ranges: []lineRange{{0, 1}},
		},
		"one-doc-no-trailing-new-line": {
			file:   `hej: du`,
			ranges: []lineRange{{0, 1}},
		},
		"two-docs": {
			file: `hej: du
jag: heter
---
arvid: hej
what-if: the joker
was: blue
`,
			ranges: []lineRange{
				{0, 2},
				{3, 6},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ranges := fileDocumentPositions(test.file)
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

//go:embed testdata/oneOf.json
var oneOf string

//go:embed testdata/anyOf.json
var anyOf string

//go:embed testdata/const.json
var const_ string

//go:embed testdata/enum.json
var enum string

//go:embed testdata/enum-with-type-string.json
var enumWithTypeString string

//go:embed testdata/enum-with-type-integer.json
var enumWithTypeInteger string

//go:embed testdata/x-kubernetes-preserve-unknown-fields.json
var xKubernetesPreserveUnknownFields string

//go:embed testdata/types.json
var types string

//go:embed testdata/refs.json
var refs string

//go:embed testdata/refs2.json
var refs2 string

//go:embed testdata/refs3.json
var refs3 string

//go:embed testdata/allOf.json
var allOf string

//go:embed testdata/anyOf-and-allOf.json
var anyOfAndAllOf string

func TestSchemaDocs(t *testing.T) {
	tests := map[string]struct {
		schema string
		docs   []SchemaProperty
	}{
		"simple": {
			schema: `{"type": "object", "properties": {"name": {"type": "string", "description": "The name of the person"}}}`,
			docs: []SchemaProperty{
				{
					Path: ".",
					Type: "object",
				},
				{
					Path:        ".name",
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
			docs: []SchemaProperty{
				{
					Path: ".",
					Type: "object",
				},
				{
					Path:        ".name",
					Description: "The name of the person",
					Type:        "string",
				},
				{
					Path:        ".riddler",
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
			docs: []SchemaProperty{
				{
					Path: ".",
					Type: "object",
				},
				{
					Path:        ".tonyz",
					Description: "Tony Zarets",
					Type:        "array",
				},
				{
					Path:        ".tonyz[]",
					Description: "An epic gamer",
					Type:        "object",
				},
				{
					Path:        ".tonyz[].producerIsHuman",
					Description: "It is true",
					Type:        "boolean",
				},
			},
		},
		"oneOf": {
			schema: oneOf,
			docs: []SchemaProperty{
				{
					Path: ".",
					Type: "object",
				},
				{
					Path:        ".port",
					Description: "The port of the service",
					Type:        "oneOf",
				},
				{
					Path:        ".port?0",
					Description: "The port name",
					Type:        "string",
				},
				{
					Path:        ".port?1",
					Description: "The port number",
					Type:        "integer",
				},
			},
		},
		"anyOf": {
			schema: anyOf,
			docs: []SchemaProperty{
				{
					Path: ".",
					Type: "object",
				},
				{
					Path:        ".port",
					Description: "The port of the service",
					Type:        "anyOf",
				},
				{
					Path:        ".port?0",
					Description: "The port name",
					Type:        "string",
				},
				{
					Path:        ".port?1",
					Description: "The port number",
					Type:        "integer",
				},
			},
		},
		"const": {
			schema: const_,
			docs: []SchemaProperty{
				{
					Path: ".",
					Type: "object",
				},
				{
					Path:        ".kind",
					Description: "The service kind",
					Type:        "const",
				},
			},
		},
		"enum": {
			schema: enum,
			docs: []SchemaProperty{
				{
					Path: ".",
					Type: "object",
				},
				{
					Path:        ".level",
					Description: "The log level",
					Type:        "enum",
					Required:    true,
				},
			},
		},
		"x-kubernetes-preserve-unknown-fields": {
			schema: xKubernetesPreserveUnknownFields,
			docs: []SchemaProperty{
				{
					Path: ".",
					Type: "object",
				},
				{
					Path:        ".anything",
					Description: "An object that can be anything",
				},
			},
		},
		"types": {
			schema: types,
			docs: []SchemaProperty{
				{
					Path: ".",
					Type: "object",
				},
				{
					Path:        ".port",
					Description: "The port of the service",
					Type:        "string, integer",
				},
			},
		},
		"refs": {
			schema: refs,
			docs: []SchemaProperty{
				{
					Path:        ".",
					Type:        "object",
					Description: "A person",
				},
				{
					Path:        ".name",
					Description: "The name of the person",
					Type:        "string",
				},
			},
		},
		"refs2": {
			schema: refs2,
			docs: []SchemaProperty{
				{
					Path:        ".",
					Type:        "object",
					Description: "A person",
				},
				{
					Path:        ".name",
					Description: "The name of the person",
					Type:        "string",
				},
			},
		},
		"refs3": {
			schema: refs3,
			docs: []SchemaProperty{
				{
					Path:        ".",
					Type:        "object",
					Description: "A person",
				},
				{
					Path:        ".name",
					Description: "The name of the person",
					Type:        "string",
				},
			},
		},
		"allOf": {
			schema: allOf,
			docs: []SchemaProperty{
				{
					Path: ".",
					Type: "allOf",
				},
				{
					Path:        ".name",
					Description: "the name of the object",
					Type:        "string",
				},
				{
					Path:        ".created_at",
					Description: "when it was created",
					Type:        "integer",
				},
			},
		},
		"anyOfAndAllOf": {
			schema: anyOfAndAllOf,
			docs: []SchemaProperty{
				{
					Path: ".",
					Type: "anyOf",
				},
				{
					Path: ".?0",
					Type: "allOf",
				},
				{
					Path:        ".?0.name",
					Description: "the name",
					Type:        "string",
				},
				{
					Path:        ".?0.created_at",
					Description: "when it was created",
					Type:        "integer",
				},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var s Schema
			if err := json.Unmarshal([]byte(test.schema), &s); err != nil {
				t.Fatal(err)
			}
			docs, err := schemaDocs([]byte(test.schema))
			if err != nil {
				t.Fatalf("got unexpected error when creating docs: %s", err)
			}
			if len(docs) != len(test.docs) {
				t.Fatalf("Expected %d properties with documentation, got %+v", len(test.docs), docs)
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
				if d.Required != expected.Required {
					t.Fatalf("Expected required to be `%t`, got `%t`", expected.Required, d.Required)
				}
			}
		})
	}
}

func TestDocumentPaths(t *testing.T) {
	tests := map[string]struct {
		document string
		paths    Paths
	}{
		"array": {
			document: `spec:
  ports:
    - port: 443
      name: https
    - port: 80
      name: http
`,
			paths: Paths{
				".":                  newRange(0, 0, 0, 0),
				".spec":              newRange(0, 0, 0, 4),
				".spec.ports":        newRange(1, 2, 1, 7),
				".spec.ports.0":      newRange(2, 4, 2, 5),
				".spec.ports.0.port": newRange(2, 6, 2, 10),
				".spec.ports.0.name": newRange(3, 6, 3, 10),
				".spec.ports.1":      newRange(4, 4, 4, 5),
				".spec.ports.1.port": newRange(4, 6, 4, 10),
				".spec.ports.1.name": newRange(5, 6, 5, 10),
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			paths := documentPaths([]byte(test.document))
			if len(paths) != len(test.paths) {
				t.Fatalf("expected %d paths, got %v", len(test.paths), paths)
			}
			for path, pos := range test.paths {
				actual, found := paths[path]
				if !found {
					t.Fatalf("expected path `%s` to exist, got %v", path, paths)
				}
				if actual != pos {
					t.Fatalf("Expected position for %s to be %v, got %v", path, pos, actual)
				}
			}
		})
	}
}

func TestFillDocument(t *testing.T) {
	tests := map[string]struct {
		schema, path, expected string
	}{
		"enum": {
			schema: enum,
			path:   ".",
			expected: `level: info
`,
		},
		"enumWithTypeString": {
			schema: enumWithTypeString,
			path:   ".",
			expected: `level: info
`,
		},
		"enumWithTypeInteger": {
			schema: enumWithTypeInteger,
			path:   ".",
			expected: `status: 200
`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			filled, err := schemaFill([]byte(test.schema), test.path)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if filled != test.expected {
				t.Fatalf("expected\n`%s`\ngot\n`%s`", test.expected, filled)
			}
		})
	}
}
