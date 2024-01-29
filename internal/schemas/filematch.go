package schemas

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
)

func NewFileMatchStore(cacheDir string) (FileMatchStore, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return FileMatchStore{}, fmt.Errorf("Could not create cache dir: %s", err)
	}
	catalog, err := getCatalog()
	if err != nil {
		return FileMatchStore{}, fmt.Errorf("Failed to get schema store catalog: %s", err)
	}
	cache := map[string]Schema{}
	cachedSchemas, err := os.ReadDir(cacheDir)
	if err != nil {
		return FileMatchStore{}, fmt.Errorf("Could not read cache dir: %s", err)
	}
	for _, file := range cachedSchemas {
		filename := path.Join(cacheDir, file.Name())
		data, err := os.ReadFile(filename)
		if err != nil {
			return FileMatchStore{}, fmt.Errorf("Failed to read schema file from cache: %s", err)
		}
		URL := ""
		for _, info := range catalog {
			if info.Name == file.Name() {
				URL = info.URL
				break
			}
		}
		cache[file.Name()] = Schema{
			Schema:   data,
			URL:      URL,
			Filename: filename,
		}
	}
	return FileMatchStore{
		CacheDir: cacheDir,
		cache:    cache,
		catalog:  catalog,
	}, nil
}

type FileMatchStore struct {
	CacheDir string
	cache    map[string]Schema
	catalog  []SchemaInfo
}

type SchemaInfo struct {
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	FileMatch []string `json:"fileMatch"`
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
	schema, found := s.cache[schemaInfo.Name]
	if found {
		return schema.Schema, nil
	}
	data, err := callTheInternet(schemaInfo.URL)
	if err != nil {
		return nil, fmt.Errorf("Failed to call the internet: %s", err)
	}
	cacheFilename := path.Join(s.CacheDir, schemaInfo.Name)
	s.cache[schemaInfo.Name] = Schema{
		Schema:   data,
		URL:      schemaInfo.URL,
		Filename: cacheFilename,
	}
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
		// Match the basename only
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
