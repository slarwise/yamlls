package schemas

import (
	"encoding/json"
	"testing"
)

var schema = map[string]any{
	"$schema":  "https://json-schema.org/draft/2020-12/schema",
	"type":     "object",
	"required": []string{"name"},
	"properties": map[string]any{
		"name": map[string]any{
			"type":    "string",
			"pattern": "^b.*$",
		},
		"status": map[string]any{
			"const": "cool",
		},
	},
}

func TestValidateJson(t *testing.T) {
	doc := []byte(`{"name": "arvid", "status": "chillin'"}`)
	errors, err := ValidateJson(schema, doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, e := range errors {
		t.Logf("details: %v", e.Details())
		t.Logf("description: %v", e.Description())
		t.Logf("field: %v", e.Field())
		t.Logf("type: %v", e.Type())
		t.Logf("value: %v", e.Value())
	}
}

func TestValidateYaml(t *testing.T) {
	doc := []byte(`name: arvid
status: chillin'`)
	errors, err := ValidateYaml(schema, doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, e := range errors {
		t.Log(e)
	}
}

var schema2 = map[string]any{
	"$schema":  "https://json-schema.org/draft/2020-12/schema",
	"type":     "object",
	"required": []string{"spec"},
	"properties": map[string]any{
		"spec": map[string]any{
			"type":                 "object",
			"required":             []string{"project"},
			"additionalProperties": false,
			"properties": map[string]any{
				"project": map[string]any{
					"type": "string",
				},
			},
		},
	},
}

func TestValidateYaml2(t *testing.T) {
	doc := []byte(`
spec:
  destination:
    name: ""
    namespace: ""
    server: ""
  project: ""
`)
	errors, err := ValidateYaml(schema2, doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, e := range errors {
		t.Log(e)
	}
}

func TestGetDescription(t *testing.T) {
	schema := map[string]any{
		"$schema":  "https://json-schema.org/draft/2020-12/schema",
		"type":     "object",
		"required": []string{"spec"},
		"properties": map[string]any{
			"spec": map[string]any{
				"type":                 "object",
				"description":          "description of spec",
				"required":             []string{"project"},
				"additionalProperties": false,
				"properties": map[string]any{
					"project": map[string]any{
						"description": "description of project",
						"type":        "string",
					},
					"yolos": map[string]any{
						"description": "description of yolos",
						"type":        "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"swag": map[string]any{
									"description": "the level of swag, cannot be combined with rizz",
									"type":        "integer",
								},
							},
						},
					},
				},
			},
		},
	}
	bytes, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	tests := map[string]struct {
		path        string
		description string
	}{
		"top-level": {
			path:        "spec",
			description: "description of spec",
		},
		"second-level": {
			path:        "spec.project",
			description: "description of project",
		},
		"not-found": {
			path:        "spec.asdf",
			description: "",
		},
		"array": {
			path:        "spec.yolos.69.swag",
			description: "the level of swag, cannot be combined with rizz",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			description := GetDescription(bytes, test.path)
			if description != test.description {
				t.Fatalf("expected description to be `%s`, got `%s`", test.description, description)
			}
		})
	}
}
