package gvkstore

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type KindApiVersionStore struct {
	schemas map[GroupVersionKind]Schema
}

type GroupVersionKind struct {
	Group   string
	Version string
	Kind    string
}

type Schema struct {
	URL    string
	Schema []byte
}

func NewKindApiVersionStore(cacheDir string) (KindApiVersionStore, error) {
	gvks, err := getGVKs()
	if err != nil {
		return KindApiVersionStore{}, err
	}
	schemas := map[GroupVersionKind]Schema{}
	for _, gvk := range gvks {
		schemas[gvk] = Schema{}
	}
	return KindApiVersionStore{schemas: schemas}, nil
}

func getGVKs() ([]GroupVersionKind, error) {
	return getKubernetesGVKs()
}

type DefinitionsResponse struct {
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

func getKubernetesGVKs() ([]GroupVersionKind, error) {
	url := "https://raw.githubusercontent.com/yannh/kubernetes-json-schema/master/master-standalone-strict/_definitions.json"
	data, err := callTheInternet(url)
	if err != nil {
		return []GroupVersionKind{}, fmt.Errorf("Failed to download kubernetes GVK definitions: %s", err)
	}
	var definitions DefinitionsResponse
	if err := json.Unmarshal(data, &definitions); err != nil {
		return []GroupVersionKind{}, fmt.Errorf("Failed to unmarshal kubernetes GVK definitions response: %s", err)
	}
	groupVersionKinds := []GroupVersionKind{}
	for _, d := range definitions.Definitions {
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
