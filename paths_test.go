package main

import "testing"

func TestDocumentPaths(t *testing.T) {
	tests := map[string]struct {
		document string
		paths    Paths
	}{
		"array": {
			document: `spec:
  ports:
    - port: 443
      name: https
    - port: 80
      name: http
`,
			paths: Paths{
				".":                  newRange(0, 0, 0, 0),
				".spec":              newRange(0, 0, 0, 4),
				".spec.ports":        newRange(1, 2, 1, 7),
				".spec.ports.0":      newRange(2, 4, 2, 5),
				".spec.ports.0.port": newRange(2, 6, 2, 10),
				".spec.ports.0.name": newRange(3, 6, 3, 10),
				".spec.ports.1":      newRange(4, 4, 4, 5),
				".spec.ports.1.port": newRange(4, 6, 4, 10),
				".spec.ports.1.name": newRange(5, 6, 5, 10),
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			paths := documentPaths(test.document)
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

func TestPathAtPosition(t *testing.T) {
	tests := map[string]struct {
		document   string
		line, char int
		path       string
		range_     Range
		found      bool
	}{
		"array": {
			document: `spec:
  ports:
    - port: 443
      name: https
    - port: 80
      name: http
`,
			line:   3,
			char:   8,
			path:   ".spec.ports.0.name",
			range_: newRange(3, 6, 3, 10),
			found:  true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			path, range_, found := pathAtPosition(test.document, test.line, test.char)
			if test.found && !found {
				t.Fatalf("expected the path to be found")
			} else if !test.found && found {
				t.Fatalf("did not expect the path to be found")
			}
			if path != test.path {
				t.Fatalf("expected path `%s`, got `%s`", test.path, path)
			}
			if range_ != test.range_ {
				t.Fatalf("expected range `%v`, got `%v`", test.range_, range_)
			}
		})
	}
}
