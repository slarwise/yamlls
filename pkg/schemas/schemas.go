package schemas

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/tidwall/gjson"
)

const (
	JSONSchemaStoreURL       = "https://json.schemastore.org"
	KubernetesSchemaStoreURL = "https://github.com/yannh/kubernetes-json-schema"
	CRDSchemaStoreURL        = "https://github.com/datreeio/CRDs-catalog"
)

func NewSchemaStore(cacheDir string, addr string) (SchemaStore, error) {
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
	}, nil
}

type SchemaStore struct {
	CacheDir string
	Cache    map[string][]byte
	URL      string
}

func GetKindApiVersion(data []byte) (string, string, bool) {
	kindPath, err := yaml.PathString("$.kind")
	if err != nil {
		panic("Could not build yaml path for kind")
	}
	apiVersionPath, err := yaml.PathString("$.apiVersion")
	if err != nil {
		panic("Could not build yaml path for apiVersion")
	}
	var kind string
	if err := kindPath.Read(bytes.NewReader(data), &kind); err != nil {
		return "", "", false
	}
	var apiVersion string
	if err := apiVersionPath.Read(bytes.NewReader(data), &apiVersion); err != nil {
		return "", "", false
	}
	return kind, apiVersion, true
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
	URL, err := url.JoinPath(s.URL, "yannh/kubernetes-json-schema/blob/master/master-standalone-strict", yannhKey+".json")
	if err != nil {
		panic(fmt.Sprintf("Could not build URL from key %s", key))
	}
	resp, err := http.Get(URL)
	if err != nil {
		panic(fmt.Errorf("Could not call the internet: %s", err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic("Could not read body")
	}
	if err = os.WriteFile(path.Join(s.CacheDir, key+".json"), body, 0644); err != nil {
		panic(fmt.Errorf("Could not write schema file: %s", err))
	}
	s.Cache[key] = body
	return body, true
}

func schemaKeyFromKindApiVersion(kind string, apiVersion string) string {
	return strings.ToLower(fmt.Sprintf("%s-%s", kind, strings.ReplaceAll(apiVersion, "/", "-")))
}

func (s *SchemaStore) GetDescriptionFromKindApiVersion(kind string, apiVersion string, yamlPath *yaml.Path) (string, bool) {
	schema, found := s.SchemaFromKindApiVersion(kind, apiVersion)
	if !found {
		return "", false
	}
	path := toSchemaPath(*yamlPath)
	path = path + ".description"
	result := gjson.Get(string(schema), path)
	if !result.Exists() {
		return "", false
	}
	return result.String(), true
}

func toSchemaPath(yamlPath yaml.Path) string {
	path := yamlPath.String()
	path = strings.TrimPrefix(path, "$.")
	path = strings.ReplaceAll(path, ".", ".properties.")
	path = "properties." + path
	return path
}
