package schemas

import (
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
		Logger:              logger,
		KindApiVersionStore: NewKindApiVersionStore(filepath.Join(cacheDir, "kubernetes")),
		FileMatchStore:      NewFileMatchStore(filepath.Join(cacheDir, "json")),
	}, nil
}

type SchemaStore struct {
	Logger              *slog.Logger
	KindApiVersionStore KindApiVersionStore
	FileMatchStore      FileMatchStore
}

type Schema struct {
	Schema   []byte
	URL      string
	Filename string
}

func (s *SchemaStore) SchemaFromFilePath(filename string) ([]byte, error) {
	schema, err := s.FileMatchStore.GetSchema(filename)
	if err != nil {
		s.Logger.Error("Failed to get filematch schema", "error", err, "filename", filename)
		return []byte{}, err
	}
	return schema, nil
}

func (s *SchemaStore) SchemaURLFromFilePath(filename string) (string, error) {
	URL, err := s.FileMatchStore.GetSchemaURL(filename)
	if err != nil {
		s.Logger.Error("Failed to get schema URL", "filename", filename, "error", err)
		return "", errors.New("Not found")
	}
	return URL, nil
}

func (s *SchemaStore) SchemaFromKindApiVersion(kind string, apiVersion string) ([]byte, bool) {
	schema, err := s.KindApiVersionStore.GetSchema(kind, apiVersion)
	if err != nil {
		s.Logger.Error("Could not fetch kind + apiVersion schema", "error", err, "kind", kind, "apiVersion", apiVersion)
		return []byte{}, false
	}
	return schema, true
}

func (s *SchemaStore) SchemaURLFromKindApiVersion(kind string, apiVersion string) string {
	return s.KindApiVersionStore.GetSchemaURL(kind, apiVersion)
}

func DocsViewerURL(schemaURL string) string {
	return "https://json-schema.app/view/" + url.PathEscape("#") + "?url=" + url.QueryEscape(schemaURL)
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
