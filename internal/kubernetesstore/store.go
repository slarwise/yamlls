package kubernetesstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type KubernetesStore struct {
	Index []GroupVersionKind
}

func NewKubernetesStore() (KubernetesStore, error) {
	url := "https://raw.githubusercontent.com/yannh/kubernetes-json-schema/master/master-standalone-strict/_definitions.json"
	data, err := callTheInternet(url)
	if err != nil {
		return KubernetesStore{}, fmt.Errorf("Failed to download schema index: %s", err)
	}
	index, err := parseIndexResponse(data)
	if err != nil {
		return KubernetesStore{}, fmt.Errorf("Failed to get schema index: %s", err)
	}
	return KubernetesStore{
		Index: index,
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

var ErrorUnknownGroupVersionKind = errors.New("Unknown group version kind")

func (s *KubernetesStore) GetSchema(group, version, kind string) ([]byte, error) {
	if !isKnownGroupVersionKind(s.Index, group, version, kind) {
		return []byte{}, ErrorUnknownGroupVersionKind
	}
	URL := buildSchemaURL(group, version, kind)
	data, err := callTheInternet(URL)
	if err != nil {
		return []byte{}, fmt.Errorf("Failed to download schema: %s", err)
	}
	return data, nil
}

func (s *KubernetesStore) GetSchemaURL(group, version, kind string) (string, error) {
	if !isKnownGroupVersionKind(s.Index, group, version, kind) {
		return "", ErrorUnknownGroupVersionKind
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
