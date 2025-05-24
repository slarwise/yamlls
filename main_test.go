package main

import (
	"testing"
)

func TestFillDocument(t *testing.T) {
	schema := `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "Test",
  "type": "object",
  "required": ["name", "age"],
  "properties": {
  	"name": {
	  "type": "string"
  	},
  	"age": {
	  "type": "integer"
  	},
  	"cat": {
  	  "type": "object",
  	  "properties": {
  	  	"friendly": {
  	  	  "type": "boolean"
  	  	},
  	  	"funny": {
  	  	  "const": "hell yeah"
  	  	},
  	  	"foods": {
  	  	  "enum": ["hot", "cold"]
  	  	}
  	  }
  	},
  	"cats": {
  	  "type": "array",
  	  "items": {
  	  	"type": "string"
  	  }
  	}
  }
}`
	actual_, err := fillDocument([]byte(schema), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	actual, ok := actual_.(map[string]any)
	if !ok {
		t.Fatalf("expected a map[string]any, got %T", actual_)
	}
	name, ok := actual["name"]
	if !ok {
		t.Fatalf("expected name to be set")
	}
	if name != "" {
		t.Fatalf("expected name to be an empty string, got %v", name)
	}
	age, ok := actual["age"]
	if !ok {
		t.Fatalf("expected age to be set")
	}
	if age != 0 {
		t.Fatalf("expected age to be 0, got %v", age)
	}
}
