package kubernetesstore

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/slarwise/yamlls/internal/cachedhttp"
	. "github.com/slarwise/yamlls/internal/errors"
)

type KubernetesStore struct {
	Index      []GroupVersionKind
	httpclient cachedhttp.CachedHttpClient
}

func NewKubernetesStore(httpclient cachedhttp.CachedHttpClient) (KubernetesStore, error) {
	url := "https://raw.githubusercontent.com/yannh/kubernetes-json-schema/master/master-standalone-strict/_definitions.json"
	data, err := httpclient.GetBody(url)
	if err != nil {
		return KubernetesStore{}, fmt.Errorf("Failed to download schema index: %s", err)
	}
	index, err := parseIndexResponse(data)
	if err != nil {
		return KubernetesStore{}, fmt.Errorf("Failed to get schema index: %s", err)
	}
	return KubernetesStore{
		Index:      index,
		httpclient: httpclient,
	}, nil
}

type GroupVersionKind struct {
	Group   string
	Version string
	Kind    string
}

type IndexResponse struct {
	Definitions map[string]GVKDefinition `json:"definitions"`
}

type GVKDefinition struct {
	XKubernetesGroupVersionKind []XKubernetesGroupVersionKind `json:"x-kubernetes-group-version-kind,omitempty"`
}

type XKubernetesGroupVersionKind struct {
	Group   string `json:"group"`
	Kind    string `json:"kind"`
	Version string `json:"version"`
}

func parseIndexResponse(data []byte) ([]GroupVersionKind, error) {
	var indexResponse IndexResponse
	if err := json.Unmarshal(data, &indexResponse); err != nil {
		return []GroupVersionKind{}, fmt.Errorf("Failed to unmarshal index response: %s", err)
	}
	groupVersionKinds := []GroupVersionKind{}
	for _, d := range indexResponse.Definitions {
		if d.XKubernetesGroupVersionKind != nil {
			for _, gvk := range d.XKubernetesGroupVersionKind {
				groupVersionKinds = append(groupVersionKinds, GroupVersionKind{
					Group:   gvk.Group,
					Version: gvk.Version,
					Kind:    gvk.Kind,
				})
			}

		}
	}
	return groupVersionKinds, nil
}

func (s *KubernetesStore) GetSchema(group, version, kind string) ([]byte, error) {
	if !isKnownGroupVersionKind(s.Index, group, version, kind) {
		return []byte{}, ErrorSchemaNotFound
	}
	URL := buildSchemaURL(group, version, kind)
	data, err := s.httpclient.GetBody(URL)
	if err != nil {
		return []byte{}, fmt.Errorf("Failed to download schema: %s", err)
	}
	return data, nil
}

func (s *KubernetesStore) GetSchemaURL(group, version, kind string) (string, error) {
	if !isKnownGroupVersionKind(s.Index, group, version, kind) {
		return "", ErrorSchemaNotFound
	}
	return buildSchemaURL(group, version, kind), nil
}

func isKnownGroupVersionKind(index []GroupVersionKind, group, version, kind string) bool {
	for _, gvk := range index {
		if group == gvk.Group && version == gvk.Version && kind == gvk.Kind {
			return true
		}
	}
	return false
}

func buildSchemaURL(group, version, kind string) string {
	basename := ""
	if group == "" {
		basename = fmt.Sprintf("%s-%s.json", strings.ToLower(kind), version)
	} else {
		basename = fmt.Sprintf("%s-%s-%s.json", strings.ToLower(kind), group, version)
	}
	return fmt.Sprintf("https://raw.githubusercontent.com/yannh/kubernetes-json-schema/master/master-standalone-strict/%s", basename)
}
