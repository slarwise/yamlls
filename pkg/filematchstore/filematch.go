package schemas

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

type SchemaInfo struct {
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	FileMatch []string `json:"fileMatch"`
}

type FileMatchStore struct {
	CacheDir string
	schemas  map[string][]byte
	catalog  []SchemaInfo
}

func NewFileMatchStore(cacheDir string) (FileMatchStore, error) {
	catalogFilename := path.Join(cacheDir, "filematch-catalog.json")
	catalog, err := readCachedCatalog(catalogFilename)
	if err != nil {
		catalog, err = getCatalog()
		if err != nil {
			return FileMatchStore{}, fmt.Errorf("Failed to get schema store catalog: %s", err)
		}
		data, err := json.Marshal(catalog)
		if err != nil {
			return FileMatchStore{}, fmt.Errorf("Failed to marshal URLs: %s", err)
		}
		if err := os.WriteFile(catalogFilename, data, 0644); err != nil {
			return FileMatchStore{}, fmt.Errorf("Could not write catalog to filesystem: %s", err)
		}
	}
	schemaDir := path.Join(cacheDir, "filematch")
	if err := os.MkdirAll(schemaDir, 0755); err != nil {
		return FileMatchStore{}, fmt.Errorf("Could not create cache dir: %s", err)
	}
	schemas, err := readCachedFilematchSchemas(schemaDir)
	if err != nil {
		return FileMatchStore{}, fmt.Errorf("Could not read schemas from filesystem: %s", err)
	}
	return FileMatchStore{
		CacheDir: cacheDir,
		schemas:  schemas,
		catalog:  catalog,
	}, nil
}

func readCachedCatalog(filename string) ([]SchemaInfo, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return []SchemaInfo{}, fmt.Errorf("Could not read catalog from filesystem: %s", err)
	}
	var catalog []SchemaInfo
	if err := json.Unmarshal(data, &catalog); err != nil {
		return []SchemaInfo{}, fmt.Errorf("Could not unmarshal catalog: %s", err)
	}
	return catalog, nil
}

func readCachedFilematchSchemas(dir string) (map[string][]byte, error) {
	schemas := make(map[string][]byte)
	schemaFiles, err := os.ReadDir(dir)
	if err != nil {
		return map[string][]byte{}, fmt.Errorf("Could not read cache dir: %s", err)
	}
	for _, file := range schemaFiles {
		filename := path.Join(dir, file.Name())
		data, err := os.ReadFile(filename)
		if err != nil {
			return map[string][]byte{}, fmt.Errorf("Could not read schema file from cache: %s", err)
		}
		key := strings.TrimSuffix(file.Name(), ".json")
		schemas[key] = data
	}
	return schemas, nil
}

func getCatalog() ([]SchemaInfo, error) {
	response, err := callTheInternet("https://www.schemastore.org/api/json/catalog.json")
	if err != nil {
		return []SchemaInfo{}, err
	}
	var catalogResponse struct {
		Schemas []SchemaInfo `json:"schemas"`
	}
	if err := json.Unmarshal(response, &catalogResponse); err != nil {
		return []SchemaInfo{}, err
	}
	return catalogResponse.Schemas, nil
}

func (s *FileMatchStore) GetSchema(filename string) ([]byte, error) {
	schemaInfo, err := s.GetSchemaInfo(filename)
	if err != nil {
		return nil, err
	}
	schema, found := s.schemas[schemaInfo.Name]
	if found {
		return schema, nil
	}
	data, err := callTheInternet(schemaInfo.URL)
	if err != nil {
		return nil, fmt.Errorf("Failed to call the internet: %s", err)
	}
	s.schemas[schemaInfo.Name] = data
	cacheFilename := path.Join(s.CacheDir, "filematch", schemaInfo.Name+".json")
	if err := os.WriteFile(cacheFilename, data, 0644); err != nil {
		return []byte{}, fmt.Errorf("Could not write schema to cache dir: %s", err)
	}
	return data, nil
}

func (s *FileMatchStore) GetSchemaURL(filename string) (string, error) {
	schemaInfo, err := s.GetSchemaInfo(filename)
	if err != nil {
		return "", err
	}
	return schemaInfo.URL, nil
}

func (s *FileMatchStore) GetSchemaInfo(filename string) (SchemaInfo, error) {
	for _, schemaInfo := range s.catalog {
		for _, pattern := range schemaInfo.FileMatch {
			if matchFilePattern(pattern, filename) {
				return schemaInfo, nil
			}
		}
	}
	return SchemaInfo{}, errors.New("Schema not found")
}

func matchFilePattern(pattern string, filename string) bool {
	match := false
	if filepath.Base(pattern) == pattern {
		filename := filepath.Base(filename)
		m, err := filepath.Match(pattern, filename)
		if err == nil {
			match = m
		}
	} else {
		m, err := doublestar.Match(pattern, filename)
		if err == nil {
			match = m
		}
	}
	return match
}
