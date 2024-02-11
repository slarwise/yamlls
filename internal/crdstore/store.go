package crdstore

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	. "github.com/slarwise/yamlls/internal/errors"
)

type CRDStore struct {
	Index []GroupVersionKind
}

type GroupVersionKind struct {
	Group   string
	Version string
	Kind    string
}

func NewCRDStore() (CRDStore, error) {
	url := "https://api.github.com/repos/datreeio/CRDs-catalog/git/trees/main?recursive=true"
	fileTreeResponse, err := callTheInternet(url)
	if err != nil {
		return CRDStore{}, fmt.Errorf("Failed to download file tree: %s", err)
	}
	index, err := parseFileTreeResponse(fileTreeResponse)
	return CRDStore{Index: index}, nil
}

func parseFileTreeResponse(response []byte) ([]GroupVersionKind, error) {
	treeResponse := struct {
		Files []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
	}{}
	if err := json.Unmarshal(response, &treeResponse); err != nil {
		return []GroupVersionKind{}, fmt.Errorf("Failed to unmarshal github tree response: %s, body: %s", err, string(response))
	}
	gvks := []GroupVersionKind{}
	for _, f := range treeResponse.Files {
		if f.Type != "blob" {
			continue
		}
		group, version, kind := getGroupVersionKindFromFilename(f.Path)
		if group == "" || version == "" || kind == "" {
			continue
		}
		gvks = append(gvks, GroupVersionKind{
			Group:   group,
			Version: version,
			Kind:    kind,
		})
	}
	return gvks, nil
}

var groupVersionKindPattern = regexp.MustCompile(`^([a-z.]+)/([a-z]+)_(\w+).json$`)

// Example: monitoring.coreos.com/alertmanager_v1.json
func getGroupVersionKindFromFilename(filename string) (string, string, string) {
	match := groupVersionKindPattern.FindStringSubmatch(filename)
	if len(match) != 4 {
		return "", "", ""
	}
	return match[1], match[3], match[2]
}

func (s *CRDStore) GetSchema(group, version, kind string) ([]byte, error) {
	kind = strings.ToLower(kind)
	if !isKnownGroupVersionKind(s.Index, group, version, kind) {
		return []byte{}, ErrorSchemaNotFound
	}
	URL := buildSchemaURL(group, version, kind)
	data, err := callTheInternet(URL)
	if err != nil {
		return []byte{}, fmt.Errorf("Failed to download schema: %s", err)
	}
	return data, nil
}

func (s *CRDStore) GetSchemaURL(group, version, kind string) (string, error) {
	kind = strings.ToLower(kind)
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
	return fmt.Sprintf("https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/%s/%s_%s.json", group, kind, version)
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
