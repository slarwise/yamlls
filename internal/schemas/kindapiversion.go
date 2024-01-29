package schemas

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
)

func NewKindApiVersionStore(cacheDir string) (KindApiVersionStore, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return KindApiVersionStore{}, fmt.Errorf("Could not create cache dir: %s", err)
	}
	URLs, err := kindApiVersionURLs()
	if err != nil {
		return KindApiVersionStore{}, fmt.Errorf("Failed to get schema URLs: %s", err)
	}
	cache := map[string]Schema{}
	cachedSchemas, err := os.ReadDir(cacheDir)
	if err != nil {
		return KindApiVersionStore{}, fmt.Errorf("Could not read cache dir: %s", err)
	}
	for _, file := range cachedSchemas {
		filename := path.Join(cacheDir, file.Name())
		data, err := os.ReadFile(filename)
		if err != nil {
			return KindApiVersionStore{}, fmt.Errorf("Failed to read schema file from cache: %s", err)
		}
		cache[file.Name()] = Schema{
			Schema:   data,
			URL:      "", // TODO: Can we get the URL here?
			Filename: filename,
		}
	}
	return KindApiVersionStore{
		CacheDir:   cacheDir,
		cache:      cache,
		schemaURLs: URLs,
	}, nil
}

type KindApiVersionStore struct {
	CacheDir   string
	cache      map[string]Schema
	schemaURLs []string
}

func (s *KindApiVersionStore) GetSchema(kind string, apiVersion string) ([]byte, error) {
	key := kindApiVersionKey(kind, apiVersion)
	schema, found := s.cache[key]
	if found {
		return schema.Schema, nil
	}
	URL := buildKindApiVersionURL(kind, apiVersion)
	if !slices.Contains(s.schemaURLs, URL) {
		return []byte{}, fmt.Errorf("Schema URL not valid: %s", URL)
	}
	data, err := callTheInternet(URL)
	if err != nil {
		return nil, err
	}
	filename := path.Join(s.CacheDir, key+".json")
	s.cache[key] = Schema{
		Schema:   data,
		URL:      URL,
		Filename: filename,
	}
	log.Printf("filename %s", filename)
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return []byte{}, fmt.Errorf("Could not write schema to cache dir: %s", err)
	}
	return data, nil
}

func kindApiVersionURLs() ([]string, error) {
	kubernetesURLs, err := getKubernetesURLs()
	if err != nil {
		return []string{}, fmt.Errorf("Failed to get kubernetes URLs: %s", err)
	}
	CRDUrls, err := getCRDURLs()
	if err != nil {
		return []string{}, fmt.Errorf("Failed to get CRD URLs: %s", err)
	}
	URLs := append(kubernetesURLs, CRDUrls...)
	return URLs, nil
}

func getKubernetesURLs() ([]string, error) {
	URL := "https://api.github.com/repos/yannh/kubernetes-json-schema/contents/master-standalone-strict"
	data, err := callTheInternet(URL)
	if err != nil {
		return []string{}, fmt.Errorf("Failed to call github contents api: %s", err)
	}
	files := []struct {
		DownloadURL string `json:"download_url"`
	}{}
	if err := json.Unmarshal(data, &files); err != nil {
		return []string{}, fmt.Errorf("Failed to unmarshal github api contents response: %s", err)
	}
	URLs := []string{}
	for _, f := range files {
		URLs = append(URLs, f.DownloadURL)
	}
	return URLs, nil
}

var crdFilePattern = regexp.MustCompile(`^\w+(\.\w+)+/.+\.json$`)

// Example: monitoring.coreos.com/alertmanager_v1.json
func isCRDFile(filename string) bool {
	return crdFilePattern.MatchString(filename)
}

func getCRDURLs() ([]string, error) {
	URL := "https://api.github.com/repos/datreeio/CRDs-catalog/git/trees/main?recursive=true"
	data, err := callTheInternet(URL)
	if err != nil {
		return []string{}, err
	}
	treeResponse := struct {
		Files []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
	}{}
	if err := json.Unmarshal(data, &treeResponse); err != nil {
		return []string{}, fmt.Errorf("Failed to unmarshal github tree response: %s, body: %s", err, string(data))
	}
	URLs := []string{}
	for _, f := range treeResponse.Files {
		if f.Type != "blob" {
			continue
		}
		if isCRDFile(f.Path) {
			URL := fmt.Sprintf("https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/%s", f.Path)
			URLs = append(URLs, URL)
		}
	}
	return URLs, nil
}

func (s *KindApiVersionStore) GetSchemaURL(kind string, apiVersion string) (string, error) {
	URL := buildKindApiVersionURL(kind, apiVersion)
	if !slices.Contains(s.schemaURLs, URL) {
		return "", fmt.Errorf("Schema URL not valid: %s", URL)
	}
	key := kindApiVersionKey(kind, apiVersion)
	cachedSchema, found := s.cache[key]
	if found && cachedSchema.URL == "" {
		cachedSchema.URL = URL
		s.cache[key] = cachedSchema
	}
	return URL, nil
}

func kindApiVersionKey(kind string, apiVersion string) string {
	return fmt.Sprintf("%s-%s", strings.ToLower(kind), strings.ReplaceAll(apiVersion, "/", "-"))
}

func buildKindApiVersionURL(kind string, apiVersion string) string {
	if isCRD(apiVersion) {
		return buildCRDURL(kind, apiVersion)
	}
	return buildKubernetesURL(kind, apiVersion)
}

func buildCRDURL(kind string, apiVersion string) string {
	kind = strings.ToLower(kind)
	splitApiVersion := strings.Split(apiVersion, "/")
	if len(splitApiVersion) != 2 {
		return ""
	}
	host := splitApiVersion[0]
	version := splitApiVersion[1]
	return fmt.Sprintf("https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/%s/%s_%s.json", host, kind, version)
}

func buildKubernetesURL(kind string, apiVersion string) string {
	apiVersion = strings.ReplaceAll(apiVersion, "/", "-")
	kind = strings.ToLower(kind)
	filename := fmt.Sprintf("%s-%s.json", kind, apiVersion)
	return fmt.Sprintf("https://raw.githubusercontent.com/yannh/kubernetes-json-schema/master/master-standalone-strict/%s", filename)
}

func isCRD(apiVersion string) bool {
	return strings.Contains(apiVersion, ".")
}
