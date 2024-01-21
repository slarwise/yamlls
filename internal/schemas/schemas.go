package schemas

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
)

const (
	JSONSchemaStoreURL       = "https://json.schemastore.org"
	KubernetesSchemaStoreURL = "https://github.com/yannh/kubernetes-json-schema"
	CRDSchemaStoreURL        = "https://github.com/datreeio/CRDs-catalog"
)

func NewSchemaStore(logger *slog.Logger, cacheDir string, addr string) (SchemaStore, error) {
	dirEntries, err := os.ReadDir(cacheDir)
	if err != nil {
		return SchemaStore{}, fmt.Errorf("Failed to read cache dir for schemas: %s", err)
	}
	cache := make(map[string][]byte)
	for _, f := range dirEntries {
		if f.IsDir() {
			continue
		}
		key, isJson := strings.CutSuffix(f.Name(), ".json")
		if !isJson {
			continue
		}
		schema, err := os.ReadFile(path.Join(cacheDir, f.Name()))
		if err != nil {
			return SchemaStore{}, fmt.Errorf("Failed to read schema: %s", err)
		}
		cache[key] = schema
	}
	return SchemaStore{
		CacheDir: cacheDir,
		Cache:    cache,
		URL:      addr,
		Logger:   logger,
	}, nil
}

type SchemaStore struct {
	CacheDir string
	Cache    map[string][]byte
	URL      string
	Logger   *slog.Logger
}

func (s *SchemaStore) SchemaFromFilePath(path string) ([]byte, error) {
	panic("Not implemented")
}

func (s *SchemaStore) SchemaFromKindApiVersion(kind string, apiVersion string) ([]byte, bool) {
	key := schemaKeyFromKindApiVersion(kind, apiVersion)
	schema, found := s.Cache[key]
	if found {
		return schema, true
	}
	yannhKey := strings.ToLower(fmt.Sprintf("%s-%s", kind, strings.ReplaceAll(apiVersion, "/", "-")))
	URL, err := url.JoinPath(s.URL, "yannh/kubernetes-json-schema/master/master-standalone-strict", yannhKey+".json")
	if err != nil {
		s.Logger.Info("Could not build URL", "key", key)
		return []byte{}, false
	}
	resp, err := http.Get(URL)
	if err != nil {
		s.Logger.Info("Could call the internet", "error", err)
		return []byte{}, false
	}
	if resp.StatusCode != 200 {
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.Logger.Info("Could not read body", "error", err)
		return []byte{}, false
	}
	if err = os.WriteFile(path.Join(s.CacheDir, key+".json"), body, 0644); err != nil {
		s.Logger.Info("Could not write schema file", "error", err)
		return []byte{}, false
	}
	s.Cache[key] = body
	return body, true
}

func schemaKeyFromKindApiVersion(kind string, apiVersion string) string {
	return strings.ToLower(fmt.Sprintf("%s-%s", kind, strings.ReplaceAll(apiVersion, "/", "-")))
}

func (s *SchemaStore) DocsViewerURL(kind string, apiVersion string) (string, error) {
	yannhKey := strings.ToLower(fmt.Sprintf("%s-%s", kind, strings.ReplaceAll(apiVersion, "/", "-")))
	schemaURL, err := url.JoinPath(s.URL, "yannh/kubernetes-json-schema/master/master-standalone-strict", yannhKey+".json")
	if err != nil {
		s.Logger.Info("Could not build URL", "kind", kind, "apiVersion", apiVersion, "error", err)
		return "", err
	}
	return "https://json-schema.app/view/" + url.PathEscape("#") + "?url=" + url.QueryEscape(schemaURL), nil

}