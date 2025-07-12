package schema2

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/xeipuuv/gojsonschema"
)

//go:embed testdata/service-v1.json
var service []byte

func TestValidateFile(t *testing.T) {
	githubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/yannh/kubernetes-json-schema/master/master-standalone-strict/_definitions.json":
			resp := map[string]any{
				"definitions": map[string]any{
					"io.k8s.api.core.v1.Service": map[string]any{
						"x-kubernetes-group-version-kind": []map[string]string{
							{
								"group":   "",
								"kind":    "Service",
								"version": "v1",
							},
						},
					},
				},
			}
			bytes, err := json.Marshal(resp)
			if err != nil {
				panic(fmt.Sprintf("failed to marshal definitions response: %v", err))
			}
			_, _ = w.Write(bytes)
		case "/datreeio/CRDs-catalog/refs/heads/main/index.yaml":
			resp := map[string]any{
				"aadpodidentity.k8s.io": []any{
					map[string]string{
						"apiVersion": "aadpodidentity.k8s.io/v1",
						"filename":   "aadpodidentity.k8s.io/azureidentity_v1.json",
						"kind":       "AzureIdentity",
						"name":       "azureidentity_v1",
					},
				},
			}
			bytes, err := yaml.Marshal(resp)
			if err != nil {
				panic(fmt.Sprintf("failed to marshal index response: %v", err))
			}
			_, _ = w.Write(bytes)
		case "/yannh/kubernetes-json-schema/master/master-standalone-strict/service-v1.json":
			_, _ = w.Write(service)
		default:
			w.WriteHeader(404)
		}
	}))

	defer githubServer.Close()
	githubRawContentsHost = githubServer.URL

	store, err := NewKubernetesStore()
	if err != nil {
		t.Fatalf("create kubernetes store: %v", err)
	}
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
					Range: newRange(4, 2, 4, 6),
					Type:  "additional_property_not_allowed",
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
					Range: newRange(9, 2, 9, 6),
					Type:  "additional_property_not_allowed",
				},
			},
		},
		"invalid-yaml": {
			file: `got punched for no: {}
reason
`,
			errors: []ValidationError{
				{
					Range: newRange(0, 0, 2, 0),
					Type:  "invalid_yaml",
				},
			},
		},
		"no-schema": {
			file: `kind: Server
apiVersion: 1990
`,
			errors: nil,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			errors := store.ValidateFile(test.file)
			if len(errors) != len(test.errors) {
				t.Fatalf("Expected %d errors, got %v", len(test.errors), errors)
			}
			for i, e := range errors {
				expected := test.errors[i]
				if e.Range != expected.Range {
					t.Fatalf("expected error at position `%v`, got `%v`", expected.Range, e.Range)
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
			ranges := getDocumentPositions(test.file)
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
		errors []jsonValidationError
	}{
		"valid": {
			schema: `{"type": "object", "properties": {"name": {"type": "string"}}}`,
			doc:    "name: arvid",
			errors: nil,
		},
		"one-error": {
			schema: `{"type": "object", "properties": {"name": {"type": "string"}}}`,
			doc:    "name: 1",
			errors: []jsonValidationError{
				{
					Field: "name",
					Type:  "invalid_type",
				},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			s := schema{gojsonschema.NewStringLoader(test.schema)}
			errors := s.validate(test.doc)
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
					Type:        "[string, integer]",
				},
			},
		},
		"refs": {
			schema: refs,
			docs: []SchemaProperty{
				{
					Path:        "name",
					Description: "The name of the person",
					Type:        "string",
				},
			},
		},
		"refs2": {
			schema: refs2,
			docs: []SchemaProperty{
				{
					Path:        "name",
					Description: "The name of the person",
					Type:        "string",
				},
			},
		},
		"refs3": {
			schema: refs3,
			docs: []SchemaProperty{
				{
					Path:        "name",
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
					Path:        ".created_at",
					Description: "when it was created",
					Type:        "integer",
				},
				{
					Path:        ".name",
					Description: "the name of the object",
					Type:        "string",
				},
			},
		},
		"anyOfAndAllOf": {
			schema: anyOfAndAllOf,
			docs: []SchemaProperty{
				{
					Path:        "?0.created_at",
					Description: "when it was created",
					Type:        "integer",
				},
				{
					Path:        "?0.name",
					Description: "the name",
					Type:        "string",
				},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// s := schema{loader: gojsonschema.NewStringLoader(test.schema)}
			var s Schema
			if err := json.Unmarshal([]byte(test.schema), &s); err != nil {
				t.Fatal(err)
			}
			docs := Docs2(s)
			t.Logf("%+v", docs)
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
			}
		})
	}
}

func TestDocumentPaths(t *testing.T) {
	tests := map[string]struct {
		document yamlDocument
		paths    paths
	}{
		"array": {
			document: `spec:
  ports:
    - port: 443
      name: https
    - port: 80
      name: http
`,
			paths: paths{
				"spec":              newRange(0, 0, 0, 4),
				"spec.ports":        newRange(1, 2, 1, 7),
				"spec.ports.0":      newRange(2, 4, 2, 5),
				"spec.ports.0.port": newRange(2, 6, 2, 10),
				"spec.ports.0.name": newRange(3, 6, 3, 10),
				"spec.ports.1":      newRange(4, 4, 4, 5),
				"spec.ports.1.port": newRange(4, 6, 4, 10),
				"spec.ports.1.name": newRange(5, 6, 5, 10),
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			paths := test.document.Paths()
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
