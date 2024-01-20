package schemas

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"
)

const cacheDir = "./test-data/schemas"

var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

func TestReadCache(t *testing.T) {
	store, err := NewSchemaStore(slog.Default(), cacheDir, "http://localhost:8080")
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
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != test.URLPath {
					t.Fatalf("Expected URL %s, got %s", test.URLPath, r.URL.Path)
				}
				w.Header().Add("Content-Type", "application/json")
				w.WriteHeader(200)
				w.Write([]byte("{}"))
			}))
			defer server.Close()
			store, _ := NewSchemaStore(logger, cacheDir, server.URL)
			_, found := store.SchemaFromKindApiVersion(test.kind, test.apiVersion)
			if found != test.found {
				t.Fatalf("Expected to find schema for kind `%s` and apiVersion `%s`", test.kind, test.apiVersion)
			}
		})
	}
}
