package schemas

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func GetKubernetesSchemaUrl(kind, apiVersion string) (string, error) {
	key := buildKey(kind, apiVersion)
	url, found := db[key]
	if !found {
		return "", fmt.Errorf("not found")
	}
	return url, nil
}

func GetApiVersions(kind string) []string {
	kind = strings.ToLower(kind)
	suffix := "-" + kind
	var apiVersions []string
	for key := range db {
		if strings.HasSuffix(key, suffix) {
			apiVersions = append(apiVersions, strings.TrimSuffix(key, suffix))
		}
	}
	return apiVersions
}

// apps/v1-deployment -> https://raw.githubusercontent.com/yannh/...
// v1-namespace -> https://raw.githubusercontent.com/yannh/...
var db map[string]string

func init() {
	if err := initDatabase(); err != nil {
		panic(fmt.Sprintf("initialize database for kubernetes schema store: %v", err))
	}
}

type DefinitionsResponse struct {
	Definitions map[string]Definition `json:"definitions"`
}

type Definition struct {
	GVK []XKubernetesGroupVersionKind `json:"x-kubernetes-group-version-kind,omitempty"`
}

type XKubernetesGroupVersionKind struct {
	Group   string `json:"group"`
	Kind    string `json:"kind"`
	Version string `json:"version"`
}

func initDatabase() error {
	db = map[string]string{}
	definitionsUrl := "https://raw.githubusercontent.com/yannh/kubernetes-json-schema/master/master-standalone-strict/_definitions.json"
	var resp DefinitionsResponse
	if err := getJson(definitionsUrl, &resp); err != nil {
		return fmt.Errorf("get definitions in yannh/kubernetes-json-schema: %v", err)
	}
	for _, d := range resp.Definitions {
		if d.GVK != nil {
			gvk := d.GVK[0]
			kind, group, version := gvk.Kind, gvk.Group, gvk.Version
			var apiVersion string
			var basename string
			if group == "" {
				basename = fmt.Sprintf("%s-%s.json", strings.ToLower(kind), version)
				apiVersion = version
			} else {
				apiVersion = fmt.Sprintf("%s/%s", group, version)
				basename = fmt.Sprintf("%s-%s-%s.json", strings.ToLower(kind), group, version)
			}
			key := buildKey(gvk.Kind, apiVersion)
			url := fmt.Sprintf("https://raw.githubusercontent.com/yannh/kubernetes-json-schema/master/master-standalone-strict/%s", basename)
			db[key] = url
		}
	}
	return nil
}

func buildKey(kind, apiVersion string) string {
	return fmt.Sprintf("%s-%s", apiVersion, strings.ToLower(kind))
}

func getJson(url string, output any) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("%v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %v", err)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s - %s", resp.Status, body)
	}
	if err := json.Unmarshal(body, output); err != nil {
		return fmt.Errorf("unmarshal body: %v", err)
	}
	return nil
}
