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
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

func NewSchemaStore(logger *slog.Logger, cacheDir string) (SchemaStore, error) {
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
		Logger:   logger,
	}, nil
}

type SchemaStore struct {
	CacheDir string
	Cache    map[string][]byte
	Logger   *slog.Logger
}

type CatalogResponse struct {
	Schemas []SchemaInfo `json:"schemas"`
}

type SchemaInfo struct {
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	FileMatch []string `json:"fileMatch"`
}

func (s *SchemaStore) SchemaFromFilePath(path string) ([]byte, error) {
	// Call https://www.schemastore.org/api/json/catalog.json to get a
	// list of schema urls and their matching names. Probably cache this
	// response.
	// Download the schema from the given url
	// HMMMM, how do we cache this? What should be the key for the schema?
	url, err := s.SchemaURLFromFilePath(path)
	if err != nil {
		return []byte{}, err
	}
	s.Logger.Info("Found url", "url", url)
	schema, err := callTheInternet(url)
	if err != nil {
		s.Logger.Error("Failed to download schema", "error", err)
		return []byte{}, err
	}
	return schema, nil
}

func (s *SchemaStore) SchemaURLFromFilePath(filepath string) (string, error) {
	schema, err := callTheInternet("https://www.schemastore.org/api/json/catalog.json")
	if err != nil {
		s.Logger.Error("Failed to download schema info", "error", err)
		return "", err
	}
	var catalog CatalogResponse
	if err = json.Unmarshal(schema, &catalog); err != nil {
		s.Logger.Error("Failed to unmarshal catalog response", "error", err, "response", string(schema))
		return "", errors.New("Not found")
	}
	url := ""
	for _, schemaInfo := range catalog.Schemas {
		for _, fileMatch := range schemaInfo.FileMatch {
			match, err := matchFilePattern(fileMatch, filepath)
			if err != nil {
				s.Logger.Error("Bad file pattern to match against", "pattern", fileMatch)
			}
			if match {
				url = schemaInfo.URL
				break
			}
		}
	}
	if url == "" {
		return "", errors.New("Not found")
	}
	return url, nil
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
	schema, err = callTheInternet(URL)
	if err != nil {
		s.Logger.Error("Could not download schema", "error", err)
		return []byte{}, false
	}
	if err = os.WriteFile(path.Join(s.CacheDir, key+".json"), schema, 0644); err != nil {
		s.Logger.Error("Could not write schema file", "error", err)
		return []byte{}, false
	}
	s.Cache[key] = schema
	return schema, true
}

func schemaKeyFromKindApiVersion(kind string, apiVersion string) string {
	kind = strings.ToLower(kind)
	apiVersion = strings.ReplaceAll(apiVersion, "/", "-")
	return fmt.Sprintf("%s-%s", kind, apiVersion)
}

func DocsViewerURL(schemaURL string) string {
	return "https://json-schema.app/view/" + url.PathEscape("#") + "?url=" + url.QueryEscape(schemaURL)
}

func (s *SchemaStore) SchemaURLFromKindApiVersion(kind string, apiVersion string) (string, error) {
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
	return schemaURL, nil
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

func callTheInternet(URL string) ([]byte, error) {
	resp, err := http.Get(URL)
	if err != nil {
		return []byte{}, err
	}
	if resp.StatusCode != 200 {
		return []byte{}, fmt.Errorf("Got non-200 status code: %s", resp.Status)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, err
	}
	return body, nil
}

func matchFilePattern(pattern string, path string) (bool, error) {
	if filepath.Base(pattern) == pattern {
		// Match the basename only
		filename := filepath.Base(path)
		match, err := filepath.Match(pattern, filename)
		if err != nil {
			return false, err
		}
		return match, nil
	}
	match, err := doublestar.Match(pattern, path)
	if err != nil {
		return false, err
	}
	return match, nil
}
