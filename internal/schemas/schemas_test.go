package schemas

import (
	"log/slog"
	"os"
	"path"
	"testing"
)

const cacheDir = "./test-data/schemas"

var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

func TestReadCache(t *testing.T) {
	store, err := NewSchemaStore(slog.Default(), cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Cache) != 1 {
		t.Fatalf("Expected 1 schema in cache, got %d", len(store.Cache))
	}
}

func TestSchemaKeyFromKindApiVersion(t *testing.T) {
	tests := map[string]struct {
		kind       string
		apiVersion string
		expected   string
	}{
		"kubernetes": {
			kind:       "Service",
			apiVersion: "v1",
			expected:   "service-v1",
		},
		"CRD": {
			kind:       "ApplicationSet",
			apiVersion: "argoproj.io/v1alpha1",
			expected:   "applicationset-argoproj.io-v1alpha1",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			key := schemaKeyFromKindApiVersion(test.kind, test.apiVersion)
			if key != test.expected {
				t.Fatalf("Expected %s, got %s", test.expected, key)
			}
		})
	}
}

func TestSchemaFromKindApiVersion(t *testing.T) {
	tests := map[string]struct {
		kind       string
		apiVersion string
		found      bool
		URLPath    string
	}{
		"exists-on-file": {
			kind:       "Service",
			apiVersion: "v1",
			found:      true,
		},
		"does-not-exist-on-file": {
			kind:       "Deployment",
			apiVersion: "apps/v1",
			found:      true,
			URLPath:    "/yannh/kubernetes-json-schema/master/master-standalone-strict/deployment-apps-v1.json"},
	}
	defer func() {
		os.Remove(path.Join(cacheDir, "deployment-apps-v1.json"))
	}()
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			store, _ := NewSchemaStore(logger, cacheDir)
			_, found := store.SchemaFromKindApiVersion(test.kind, test.apiVersion)
			if found != test.found {
				t.Fatalf("Expected to find schema for kind `%s` and apiVersion `%s`", test.kind, test.apiVersion)
			}
		})
	}
}

func TestMatchFilePattern(t *testing.T) {
	tests := map[string]struct {
		pattern string
		name    string
		match   bool
	}{
		"basename-exact-match": {
			pattern: "test.yaml",
			name:    "/home/user1/dir/test.yaml",
			match:   true,
		},
		"basename-non-match": {
			pattern: "test.yaml",
			name:    "/home/user1/dir/test1.yaml",
			match:   false,
		},
		"basename-glob": {
			pattern: "appsettings.*.json",
			name:    "appsettings.test.json",
			match:   true,
		},
		"full-path-glob": {
			pattern: "**/.github/workflows/*.yaml",
			name:    "/home/user1/myproject/.github/workflows/build.yaml",
			match:   true,
		},
		"full-path-glob-no-match": {
			pattern: "**/.github/workflows/*.yaml",
			name:    "/home/user1/myproject/.github/test/workflows/build.yaml",
			match:   false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			match, err := matchFilePattern(test.pattern, test.name)
			if err != nil {
				t.Fatalf("Did not expect an error: %s", err)
			}
			if match != test.match {
				t.Fatalf("Expected match to be %t, got %t\n", test.match, match)
			}
		})
	}
}

func TestSchemaFromFilePath(t *testing.T) {
	s, err := NewSchemaStore(logger, cacheDir)
	if err != nil {
		t.Fatalf("Failed to create schema store: %s", err)
	}
	filePath := "/home/user1/myproject/.github/workflows/build.yaml"
	schema, err := s.SchemaFromFilePath(filePath)
	if err != nil {
		t.Fatalf("Failed to retreive schema: %s", err)
	}
	if schema == nil {
		t.Fatalf("Expected schema to include something")
	}
}
