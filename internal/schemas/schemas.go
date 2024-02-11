package schemas

import (
	"fmt"
	"net/url"

	"github.com/slarwise/yamlls/internal/crdstore"
	. "github.com/slarwise/yamlls/internal/errors"
	"github.com/slarwise/yamlls/internal/jsonschemastore"
	"github.com/slarwise/yamlls/internal/kubernetesstore"
	"github.com/slarwise/yamlls/internal/parser"
)

type SchemaStore struct {
	kubernetesStore kubernetesstore.KubernetesStore
	crdStore        crdstore.CRDStore
	jsonSchemaStore jsonschemastore.JsonSchemaStore
}

func NewSchemaStore() (SchemaStore, error) {
	kubernetesStore, err := kubernetesstore.NewKubernetesStore()
	if err != nil {
		return SchemaStore{}, fmt.Errorf("Could not create kubernetes schema store: %s", err)
	}
	crdStore, err := crdstore.NewCRDStore()
	if err != nil {
		return SchemaStore{}, fmt.Errorf("Could not create CRD schema store: %s", err)
	}
	jsonSchemaStore, err := jsonschemastore.NewJsonSchemaStore()
	if err != nil {
		return SchemaStore{}, fmt.Errorf("Could not create json schema store: %s", err)
	}
	return SchemaStore{
		kubernetesStore: kubernetesStore,
		crdStore:        crdStore,
		jsonSchemaStore: jsonSchemaStore,
	}, nil
}

func (s *SchemaStore) GetSchema(filename string, text string) ([]byte, error) {
	group, version, kind := parser.GetGroupVersionKind(text)
	if version != "" && kind != "" {
		schema, err := s.kubernetesStore.GetSchema(group, version, kind)
		if err == nil {
			return schema, nil
		}
		if err != ErrorSchemaNotFound {
			return []byte{}, fmt.Errorf("Error when fetching schema: %s", err)
		}
		schema, err = s.crdStore.GetSchema(group, version, kind)
		if err == nil {
			return schema, nil
		}
		if err != ErrorSchemaNotFound {
			return []byte{}, fmt.Errorf("Error when fetching schema: %s", err)
		}
	}
	schema, err := s.jsonSchemaStore.GetSchema(filename)
	switch err {
	case nil:
		return schema, nil
	case ErrorSchemaNotFound:
		return []byte{}, ErrorSchemaNotFound
	default:
		return []byte{}, fmt.Errorf("Error when fetching schema: %s", err)
	}
}

func (s *SchemaStore) GetSchemaURL(filename string, text string) (string, error) {
	group, version, kind := parser.GetGroupVersionKind(text)
	if group != "" && version != "" && kind != "" {
		URL, err := s.kubernetesStore.GetSchemaURL(group, version, kind)
		if err == nil {
			return URL, nil
		}
		URL, err = s.crdStore.GetSchemaURL(group, version, kind)
		if err == nil {
			return URL, nil
		}
	}
	URL, err := s.jsonSchemaStore.GetSchemaURL(filename)
	if err == nil {
		return URL, nil
	}
	return "", ErrorSchemaNotFound
}

func DocsViewerURL(schemaURL string) string {
	return "https://json-schema.app/view/" + url.PathEscape("#") + "?url=" + url.QueryEscape(schemaURL)
}
