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

func TestReadCache(t *testing.T) {
	store, err := NewSchemaStore(slog.Default(), cacheDir, "http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Cache) != 1 {
		t.Fatalf("Expected 1 schema in cache, got %d", len(store.Cache))
	}
}

func TestGetKindApiVersion(t *testing.T) {
	tests := map[string]struct {
		data       []byte
		kind       string
		apiVersion string
		found      bool
	}{
		"kubernetes": {
			data:       []byte("kind: Service\napiVersion: v1"),
			kind:       "Service",
			apiVersion: "v1",
			found:      true,
		},
		"CRD": {
			data:       []byte("kind: ApplicationSet\napiVersion: argoproj.io/v1alpha1"),
			kind:       "ApplicationSet",
			apiVersion: "argoproj.io/v1alpha1",
			found:      true,
		},
		"no-api-version": {
			data:       []byte("kind: ApplicationSet\n"),
			kind:       "",
			apiVersion: "",
			found:      false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			kind, apiVersion, found := GetKindApiVersion(test.data)
			if found != test.found {
				t.Fatal("Expected to find kind and apiVersion")
			}
			if kind != test.kind {
				t.Fatalf("Expected kind to be %s, got %s", test.kind, kind)
			}
			if apiVersion != test.apiVersion {
				t.Fatalf("Expected apiVersion to be %s, got %s", test.apiVersion, apiVersion)
			}
		})
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
			store, _ := NewSchemaStore(slog.Default(), cacheDir, server.URL)
			_, found := store.SchemaFromKindApiVersion(test.kind, test.apiVersion)
			if found != test.found {
				t.Fatalf("Expected to find schema for kind `%s` and apiVersion `%s`", test.kind, test.apiVersion)
			}
		})
	}
}

func TestGetDescription(t *testing.T) {
	yamlPath := "$.spec.ports"
	store, err := NewSchemaStore(slog.Default(), cacheDir, "")
	if err != nil {
		t.Fatalf("Could not create schema store: %s", err)
	}
	description, found := store.GetDescriptionFromKindApiVersion("service", "v1", yamlPath)
	if !found {
		t.Fatal("Expected to find description")
	}
	expected := "The list of ports that are exposed by this service. More info: https://kubernetes.io/docs/concepts/services-networking/service/#virtual-ips-and-service-proxies"
	if description != expected {
		t.Fatalf("Expected %s, got %s", expected, description)
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
