package ast

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
	line := 3
	column := 4
	path, err := GetPathAtPosition(line, column, yml)
	if err != nil {
		t.Fatal(err)
	}
	if path != "$.spec" {
		t.Fatalf("Expected path to be `$.spec`, got %s\n", path)
	}
}
