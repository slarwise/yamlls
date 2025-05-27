package parser

import (
	"testing"
)

func TestPathsToPositions(t *testing.T) {
	tests := map[string]struct {
		doc      []byte
		expected map[string]Position
	}{
		"simple": {
			doc: []byte(`name: arvid
status: chillin'
cat:
  name: strimma
  nice: true`),
			expected: map[string]Position{
				"name":     {0, 0, 4},
				"status":   {1, 0, 6},
				"cat":      {2, 0, 3},
				"cat.name": {3, 2, 6},
				"cat.nice": {4, 2, 6},
			},
		},
		"argocd-app": {
			doc: []byte(`kind: Application
spec:
  destination:
    name: ""
    namespace: ""
    server: ""
  project: ""
`),
			expected: map[string]Position{
				"kind":                       {0, 0, 4},
				"spec":                       {1, 0, 4},
				"spec.destination":           {2, 2, 13},
				"spec.destination.name":      {3, 4, 8},
				"spec.destination.namespace": {4, 4, 13},
				"spec.destination.server":    {5, 4, 10},
				"spec.project":               {6, 2, 9},
			},
		},
		"service": {
			doc: []byte(`kind: Service
apiVersion: v1
metadata:
  name: hej
spec:
  ports:
    - port: 8080
`),
			expected: map[string]Position{
				"kind":          {0, 0, 4},
				"apiVersion":    {1, 0, 10},
				"metadata":      {2, 2, 13},
				"metadata.name": {3, 4, 8},
				"spec":          {4, 4, 13},
				"spec.ports":    {5, 4, 10},
				"spec.ports.0":  {6, 2, 9},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := PathsToPositions(test.doc)
			t.Log(result)
			if err != nil {
				t.Logf("unexpected error: %v", err)
			}
			if len(result) != len(test.expected) {
				t.Fatalf("expected %d paths, got %d", len(test.expected), len(result))
			}
			for path, position := range test.expected {
				actualPosition, found := result[path]
				if !found {
					t.Fatalf("expected path `%s` to exist", path)
				}
				if position != actualPosition {
					t.Fatalf("expected position for path `%s` to be %v, got %v", path, position, actualPosition)
				}
			}

		})
	}
}

func TestPathAtPosition(t *testing.T) {
	doc := []byte(`name: arvid
status: chillin'
cat:
  name: strimma
  nice: true`)
	tests := map[string]struct {
		line, col int
		expected  string
	}{
		"left": {
			line:     0,
			col:      0,
			expected: "name",
		},
		"right": {
			line:     0,
			col:      3,
			expected: "name",
		},
		"bottom": {
			line:     4,
			col:      5,
			expected: "cat.nice",
		},
		"out-of-range": {
			line:     5,
			col:      0,
			expected: "",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			path, err := PathAtPosition(doc, test.line, test.col)
			if err != nil {
				t.Logf("unexpected error: %v", err)
			}
			if path != test.expected {
				t.Fatalf("expected path to be `%s`, got `%s`", test.expected, path)
			}
		})
	}
}

func TestUpdateDocument(t *testing.T) {
	doc := []byte(`
kind: Server
apiVersion: 1996
# hello
uptime: 69
`)
	path := "uptime"
	replacement := []byte("70")
	updated, err := ReplaceNode(doc, path, replacement)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Log(updated)
}

func TestGetKindAndApiVersion(t *testing.T) {
	tests := map[string]struct {
		doc              []byte
		kind, apiVersion string
		err              bool
	}{
		"both-kind-and-apiVersion": {
			doc: []byte(`
kind: Server
apiVersion: 1990
		`),
			kind:       "Server",
			apiVersion: "1990",
			err:        false,
		},
		"kind-only": {
			doc: []byte(`
kind: Server
		`),
			kind:       "Server",
			apiVersion: "",
			err:        false,
		},
		"apiVersion-only": {
			doc: []byte(`
apiVersion: 1990
		`),
			kind:       "",
			apiVersion: "1990",
			err:        false,
		},
		"invalid-yaml": {
			doc: []byte(`
kind
apiVersion
`),
			kind:       "",
			apiVersion: "",
			err:        true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			kind, apiVersion, err := GetKindAndApiVersion(test.doc)
			if kind != test.kind {
				t.Fatalf("expected kind to be `%s`, got `%s`", test.kind, kind)
			}
			if apiVersion != test.apiVersion {
				t.Fatalf("expected apiVersion to be `%s`, got `%s`", test.apiVersion, apiVersion)
			}
			if test.err && err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !test.err && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
