package schemas

import (
	"encoding/json"
	"errors"
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

type CatalogResponse struct {
	Schemas []SchemaInfo `json:"schemas"`
}

type SchemaInfo struct {
	URL       string   `json:"url"`
	FileMatch []string `json:"fileMatch"`
}

func (s *SchemaStore) SchemaFromFilePath(path string) ([]byte, error) {
	// Call https://www.schemastore.org/api/json/catalog.json to get a
	// list of schema urls and their matching names. Probably cache this
	// response.
	// Download the schema from the given url
	resp, err := http.Get("https://www.schemastore.org/api/json/catalog.json")
	if err != nil {
		s.Logger.Error("Failed to call the internet", "error", err)
		return []byte{}, err
	}
	if resp.StatusCode != 200 {
		s.Logger.Error("Got non-200 response from json schema store", "status", resp.Status)
		return []byte{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.Logger.Error("Failed to read body in catalog response", "error", err)
		return []byte{}, err
	}
	var catalog CatalogResponse
	if err = json.Unmarshal(body, &catalog); err != nil {
		s.Logger.Error("Failed to unmarshal catalog response", "error", err, "body", string(body))
	}
	schemas := catalog.Schemas
	url := ""
	for _, s := range schemas {
		for _, fileMatch := range s.FileMatch {
			// TODO: Match properly. Globs?
			if strings.Contains(path, fileMatch) {
				url = s.URL
				break
			}
		}
	}
	resp, err = http.Get(url)
	if err != nil {
		s.Logger.Error("Failed to call the internet", "error", err)
		return []byte{}, err
	}
	if resp.StatusCode != 200 {
		s.Logger.Error("Got non-200 response from json schema store", "status", resp.Status)
		return []byte{}, err
	}
	defer resp.Body.Close()
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		s.Logger.Error("Failed to read schema body", "error", err)
		return []byte{}, err
	}
	return body, nil
}

func (s *SchemaStore) SchemaFromKindApiVersion(kind string, apiVersion string) ([]byte, bool) {
	key := schemaKeyFromKindApiVersion(kind, apiVersion)
	schema, found := s.Cache[key]
	if found {
		return schema, true
	}
	URL := ""
	var err error
	if isCRD(apiVersion) {
		URL, err = buildCRDURL(kind, apiVersion)
		if err != nil {
			s.Logger.Error("Could not build CRD URL", "error", err, "kind", kind, "apiVersion", apiVersion)
			return []byte{}, false
		}
	} else {
		URL = buildKubernetesURL(kind, apiVersion)
	}
	resp, err := http.Get(URL)
	if err != nil {
		s.Logger.Error("Could call the internet", "error", err)
		return []byte{}, false
	}
	if resp.StatusCode != 200 {
		s.Logger.Error("Got non-200 status code from schema repo", "error", err)
		return []byte{}, false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.Logger.Error("Could not read body", "error", err)
		return []byte{}, false
	}
	if err = os.WriteFile(path.Join(s.CacheDir, key+".json"), body, 0644); err != nil {
		s.Logger.Error("Could not write schema file", "error", err)
		return []byte{}, false
	}
	s.Cache[key] = body
	return body, true
}

func schemaKeyFromKindApiVersion(kind string, apiVersion string) string {
	kind = strings.ToLower(kind)
	apiVersion = strings.ReplaceAll(apiVersion, "/", "-")
	return fmt.Sprintf("%s-%s", kind, apiVersion)
}

func (s *SchemaStore) DocsViewerURL(kind string, apiVersion string) (string, error) {
	schemaURL := ""
	var err error
	if isCRD(apiVersion) {
		schemaURL, err = buildCRDURL(kind, apiVersion)
		if err != nil {
			s.Logger.Error("Could not build CRD URL", "error", err, "kind", kind, "apiVersion", apiVersion)
			return "", errors.New("Not found")
		}
	} else {
		schemaURL = buildKubernetesURL(kind, apiVersion)
	}
	return "https://json-schema.app/view/" + url.PathEscape("#") + "?url=" + url.QueryEscape(schemaURL), nil
}

func isCRD(apiVersion string) bool {
	return strings.Contains(apiVersion, ".")
}

func buildKubernetesURL(kind string, apiVersion string) string {
	apiVersion = strings.ReplaceAll(apiVersion, "/", "-")
	kind = strings.ToLower(kind)
	filename := fmt.Sprintf("%s-%s.json", kind, apiVersion)
	return fmt.Sprintf("https://raw.githubusercontent.com/yannh/kubernetes-json-schema/master/master-standalone-strict/%s", filename)
}

func buildCRDURL(kind string, apiVersion string) (string, error) {
	kind = strings.ToLower(kind)
	splitApiVersion := strings.Split(apiVersion, "/")
	if len(splitApiVersion) != 2 {
		return "", errors.New("CRD apiVersion must contain exactly 1 `/`")
	}
	host := splitApiVersion[0]
	version := splitApiVersion[1]
	return fmt.Sprintf("https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/%s/%s_%s.json", host, kind, version), nil
}
