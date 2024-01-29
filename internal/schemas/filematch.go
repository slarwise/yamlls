package schemas

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
)

func NewFileMatchStore(cacheDir string) FileMatchStore {
	return FileMatchStore{
		CacheDir: cacheDir,
		cache:    map[string]Schema{},
	}
}

type FileMatchStore struct {
	CacheDir string
	cache    map[string]Schema
}

type CatalogResponse struct {
	Schemas []SchemaInfo `json:"schemas"`
}

type SchemaInfo struct {
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	FileMatch []string `json:"fileMatch"`
}

func (s *FileMatchStore) GetSchema(filename string) ([]byte, error) {
	// Call https://www.schemastore.org/api/json/catalog.json to get a
	// list of schema urls and their matching names. Probably cache this
	// response.
	// Download the schema from the given url
	// HMMMM, how do we cache this? What should be the key for the schema?
	URL, err := s.GetSchemaURL(filename)
	if err != nil {
		return []byte{}, err
	}
	schema, err := callTheInternet(URL)
	if err != nil {
		return []byte{}, fmt.Errorf("Failed to call the internet: %s", err)
	}
	// TODO: Cache the schema
	return schema, nil
}

func (s *FileMatchStore) GetSchemaURL(filename string) (string, error) {
	response, err := callTheInternet("https://www.schemastore.org/api/json/catalog.json")
	if err != nil {
		return "", err
	}
	var catalogResponse CatalogResponse
	if err := json.Unmarshal(response, &catalogResponse); err != nil {
		return "", err
	}
	URL := ""
	for _, s := range catalogResponse.Schemas {
		for _, pattern := range s.FileMatch {
			if matchFilePattern(pattern, filename) {
				URL = s.URL
			}
		}
	}
	if URL == "" {
		return "", errors.New("Schema not found")
	}
	return URL, nil
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
