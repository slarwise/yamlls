package schemas

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
)

func NewSchemaStore(logger *slog.Logger, cacheDir string) (SchemaStore, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return SchemaStore{}, err
	}
	kindApiVersionStore, err := NewKindApiVersionStore(cacheDir)
	if err != nil {
		return SchemaStore{}, err
	}
	fileMatchStore, err := NewFileMatchStore(cacheDir)
	if err != nil {
		return SchemaStore{}, err
	}
	return SchemaStore{
		Logger:              logger,
		KindApiVersionStore: kindApiVersionStore,
		FileMatchStore:      fileMatchStore,
	}, nil
}

type SchemaStore struct {
	Logger              *slog.Logger
	KindApiVersionStore KindApiVersionStore
	FileMatchStore      FileMatchStore
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

func (s *SchemaStore) SchemaURLFromKindApiVersion(kind string, apiVersion string) (string, error) {
	URL, err := s.KindApiVersionStore.GetSchemaURL(kind, apiVersion)
	if err != nil {
		return "", errors.New("Schema not found")
	}
	return URL, nil
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
