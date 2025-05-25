package parser

import "testing"

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
