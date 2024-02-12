package parser

import (
	"testing"
)

const deployment = `kind: Deployment
apiVersion: apps/v1
spec:
  metadata:
    labels:
      app: myapp`

const service = `apiVersion: v1
kind: Service
metadata:
  name: myservice
  labels:
    app: myapp
spec:
  ports:
    - name: http
      port: 80
      targetPort: http
  selector:
    app: APP`

func TestGetPathAtPosition(t *testing.T) {
	var line uint32 = 2
	var column uint32 = 3
	path, err := GetPathAtPosition(line, column, deployment)
	if err != nil {
		t.Fatal(err)
	}
	if path != "$.spec" {
		t.Fatalf("Expected path to be `$.spec`, got %s\n", path)
	}
}

func TestGetPositionForPath(t *testing.T) {
	tests := map[string]struct {
		path                   string
		line, startCol, endCol uint32
	}{
		"simple": {
			path:     "$.metadata.labels.app",
			line:     5,
			startCol: 4,
			endCol:   7,
		},
		"list": {
			path:     "$.spec.ports[0].port",
			line:     9,
			startCol: 6,
			endCol:   10,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			line, startCol, endCol, err := GetPositionForPath(test.path, service)
			if err != nil {
				t.Fatalf("Did not expect an error, got %s", err)
			}
			if line != test.line {
				t.Fatalf("Expected line to be %d, got %d", test.line, line)
			}
			if startCol != test.startCol {
				t.Fatalf("Expected startCol to be %d, got %d", test.startCol, startCol)
			}
			if endCol != test.endCol {
				t.Fatalf("Expected endCol to be %d, got %d", test.endCol, endCol)
			}
		})
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
