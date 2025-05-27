package parser

import (
	"testing"
)

func TestPathsToPositions(t *testing.T) {
	doc := []byte(`name: arvid
status: chillin'
cat:
  name: strimma
  nice: true`)
	result, err := PathsToPositions(doc)
	if err != nil {
		t.Logf("unexpected error: %v", err)
	}
	expected := map[string]Position{
		"name":     {1, 1, 5},
		"status":   {2, 1, 7},
		"cat":      {3, 1, 4},
		"cat.name": {4, 3, 7},
		"cat.nice": {5, 3, 7},
	}
	if len(result) != len(expected) {
		t.Fatalf("expected %d paths, got %d", len(expected), len(result))
	}
	for path, position := range expected {
		actualPosition, found := result[path]
		if !found {
			t.Fatalf("expected path `%s` to exist", path)
		}
		if position != actualPosition {
			t.Fatalf("expected position for path `%s` to be %v, got %v", path, position, actualPosition)
		}
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
			line:     1,
			col:      1,
			expected: "name",
		},
		"right": {
			line:     1,
			col:      4,
			expected: "name",
		},
		"bottom": {
			line:     5,
			col:      6,
			expected: "cat.nice",
		},
		"out-of-range": {
			line:     6,
			col:      1,
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
