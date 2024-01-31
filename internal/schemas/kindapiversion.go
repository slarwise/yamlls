package schemas

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
)

type KindApiVersionStore struct {
	CacheDir string
	schemas  kindApiVersionToSchema
	urls     []string
}

type kindApiVersionToSchema map[string][]byte

func NewKindApiVersionStore(cacheDir string) (KindApiVersionStore, error) {
	urlsFilename := path.Join(cacheDir, "kindapiversion-urls.json")
	URLs, err := readCachedURLs(urlsFilename)
	if err != nil {
		URLs, err = kindApiVersionURLs()
		if err != nil {
			return KindApiVersionStore{}, fmt.Errorf("Failed to get URLs: %s", err)
		}
		data, err := json.Marshal(URLs)
		if err != nil {
			return KindApiVersionStore{}, fmt.Errorf("Failed to marshal URLs: %s", err)
		}
		if err := os.WriteFile(urlsFilename, data, 0644); err != nil {
			return KindApiVersionStore{}, fmt.Errorf("Failed to write URLs to filesystem: %s", err)
		}
	}
	schemaDir := path.Join(cacheDir, "kindapiversion")
	if err := os.MkdirAll(schemaDir, 0755); err != nil {
		return KindApiVersionStore{}, fmt.Errorf("Failed to create schema directory: %s", err)
	}
	schemas, err := readCachedKindApiVersionSchemas(schemaDir)
	if err != nil {
		return KindApiVersionStore{}, fmt.Errorf("Failed to read schemas from filesystem: %s", err)
	}
	return KindApiVersionStore{
		CacheDir: cacheDir,
		schemas:  schemas,
		urls:     URLs,
	}, nil
}

func readCachedURLs(filename string) ([]string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return []string{}, err
	}
	var URLs []string
	if err := json.Unmarshal(data, &URLs); err != nil {
		return []string{}, err
	}
	return URLs, nil
}

func readCachedKindApiVersionSchemas(dir string) (kindApiVersionToSchema, error) {
	schemas := kindApiVersionToSchema{}
	schemaFiles, err := os.ReadDir(dir)
	if err != nil {
		return kindApiVersionToSchema{}, fmt.Errorf("Could not read cache dir: %s", err)
	}
	for _, file := range schemaFiles {
		filename := path.Join(dir, file.Name())
		data, err := os.ReadFile(filename)
		if err != nil {
			return kindApiVersionToSchema{}, fmt.Errorf("Failed to read schema file from cache: %s", err)
		}
		basenameNoExt := strings.TrimSuffix(file.Name(), ".json")
		split := strings.Split(basenameNoExt, "-")
		if len(split) != 2 {
			return kindApiVersionToSchema{}, fmt.Errorf("Failed to parse schema filename into kind and apiVersion: %s", file.Name())
		}
		kind := split[0]
		apiVersion := split[1]
		apiVersion = strings.ReplaceAll(apiVersion, "!", "/")
		key := fmt.Sprintf("%s-%s", kind, apiVersion)
		schemas[key] = data
	}
	return schemas, nil
}

func (s *KindApiVersionStore) GetSchema(kind string, apiVersion string) ([]byte, error) {
	key := fmt.Sprintf("%s-%s", kind, apiVersion)
	schema, found := s.schemas[key]
	if found {
		return schema, nil
	}
	URL := buildKindApiVersionURL(kind, apiVersion)
	if !slices.Contains(s.urls, URL) {
		return nil, fmt.Errorf("The URL is not valid: %s", URL)
	}
	data, err := callTheInternet(URL)
	if err != nil {
		return nil, fmt.Errorf("Failed to download schema: %s", err)
	}
	s.schemas[key] = data
	basename := fmt.Sprintf("%s-%s.json", kind, strings.ReplaceAll(apiVersion, "/", "!"))
	filename := path.Join(s.CacheDir, "kindapiversion", basename)
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
	if !slices.Contains(s.urls, URL) {
		return "", fmt.Errorf("Schema URL not valid: %s", URL)
	}
	return URL, nil
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
