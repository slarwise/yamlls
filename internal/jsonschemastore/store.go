package jsonschemastore

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/slarwise/yamlls/internal/cachedhttp"
	. "github.com/slarwise/yamlls/internal/errors"

	"github.com/bmatcuk/doublestar/v4"
)

type SchemaInfo struct {
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	FileMatch []string `json:"fileMatch"`
}

type JsonSchemaStore struct {
	Index      []SchemaInfo
	httpclient cachedhttp.CachedHttpClient
}

func NewJsonSchemaStore(httpclient cachedhttp.CachedHttpClient) (JsonSchemaStore, error) {
	indexResponse, err := httpclient.GetBody("https://www.schemastore.org/api/json/catalog.json")
	if err != nil {
		return JsonSchemaStore{}, fmt.Errorf("Failed to download index: %s", err)
	}
	index, err := parseIndexResponse(indexResponse)
	if err != nil {
		return JsonSchemaStore{}, fmt.Errorf("Failed to parse index: %s", err)
	}
	return JsonSchemaStore{
		Index:      index,
		httpclient: httpclient,
	}, nil
}

func parseIndexResponse(data []byte) ([]SchemaInfo, error) {
	var indexResponse struct {
		Schemas []SchemaInfo `json:"schemas"`
	}
	if err := json.Unmarshal(data, &indexResponse); err != nil {
		return []SchemaInfo{}, fmt.Errorf("Failed to unmarshal index response: %s", err)
	}
	return indexResponse.Schemas, nil
}

func (s *JsonSchemaStore) GetSchema(filename string) ([]byte, error) {
	schemaInfo, found := getMatchingSchemaInfo(s.Index, filename)
	if !found {
		return nil, ErrorSchemaNotFound
	}
	data, err := s.httpclient.GetBody(schemaInfo.URL)
	if err != nil {
		return nil, fmt.Errorf("Failed to call the internet: %s", err)
	}
	return data, nil
}

func (s *JsonSchemaStore) GetSchemaURL(filename string) (string, error) {
	schemaInfo, found := getMatchingSchemaInfo(s.Index, filename)
	if !found {
		return "", ErrorSchemaNotFound
	}
	return schemaInfo.URL, nil
}

func getMatchingSchemaInfo(index []SchemaInfo, filename string) (SchemaInfo, bool) {
	for _, schemaInfo := range index {
		for _, pattern := range schemaInfo.FileMatch {
			if matchFilePattern(pattern, filename) {
				return schemaInfo, true
			}
		}
	}
	return SchemaInfo{}, false
}

func matchFilePattern(pattern string, filename string) bool {
	match := false
	if filepath.Base(pattern) == pattern {
		// The pattern only matches on the basename
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
