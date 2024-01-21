package parser

import (
	"testing"
)

const yml = `kind: Deployment
apiVersion: apps/v1
spec:
  metadata:
    labels:
      app: myapp`

func TestGetTokenAtPosition(t *testing.T) {
	var line uint32 = 3
	var column uint32 = 4
	path, err := GetPathAtPosition(line, column, yml)
	if err != nil {
		t.Fatal(err)
	}
	if path != "$.spec" {
		t.Fatalf("Expected path to be `$.spec`, got %s\n", path)
	}
}

func TestToSchemaPath(t *testing.T) {
	yamlPath := "$.spec.ports"
	schemaPath := toSchemaPath(yamlPath)
	expected := "properties.spec.properties.ports"
	if schemaPath != expected {
		t.Fatalf("Expected %s, got %s", expected, schemaPath)
	}
}
