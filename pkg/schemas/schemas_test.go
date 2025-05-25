package schemas

import "testing"

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
