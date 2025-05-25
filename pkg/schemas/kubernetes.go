package schemas

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/goccy/go-yaml"
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
	var apiVersions []string
	for key := range db {
		currentKind, apiVersion := usMapleKey(key)
		if currentKind == kind {
			apiVersions = append(apiVersions, apiVersion)
		}
	}
	return apiVersions
}

func PrintSchemas() {
	for key, url := range db {
		fmt.Println(key, url)
	}
}

// apps/v1-deployment -> https://raw.githubusercontent.com/yannh/...
// v1-namespace -> https://raw.githubusercontent.com/yannh/...
var db map[string]string

func init() {
	if err := initDatabase(); err != nil {
		panic(fmt.Sprintf("initialize database for kubernetes schema store: %v", err))
	}
}

func initDatabase() error {
	db = map[string]string{}
	nativeResources, err := getNativeResourceDefinitions()
	if err != nil {
		return fmt.Errorf("get native resource definitions: %v", err)
	}
	crds, err := getCustomResourceDefinitions()
	if err != nil {
		return fmt.Errorf("get custom resource definitions: %v", err)
	}

	allResources := append(nativeResources, crds...)
	for _, resource := range allResources {
		key := buildKey(resource.Kind, resource.ApiVersion)
		db[key] = resource.Url
	}
	return nil
}

type Resource struct{ Kind, ApiVersion, Url string }

type DefinitionsResponse struct {
	Definitions map[string]Definition `json:"definitions"`
}

type Definition struct {
	GVK []GroupVersionKind `json:"x-kubernetes-group-version-kind,omitempty"`
}

type GroupVersionKind struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

func getNativeResourceDefinitions() ([]Resource, error) {
	definitionsUrl := "https://raw.githubusercontent.com/yannh/kubernetes-json-schema/master/master-standalone-strict/_definitions.json"
	var resp DefinitionsResponse
	if err := getJson(definitionsUrl, &resp); err != nil {
		return nil, fmt.Errorf("get definitions in yannh/kubernetes-json-schema: %v", err)
	}
	var resources []Resource
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
				group := strings.Split(group, ".")[0]
				apiVersion = fmt.Sprintf("%s/%s", group, version)
				basename = fmt.Sprintf("%s-%s-%s.json", strings.ToLower(kind), group, version)
			}
			url := fmt.Sprintf("https://raw.githubusercontent.com/yannh/kubernetes-json-schema/master/master-standalone-strict/%s", basename)
			resources = append(resources, Resource{
				Kind:       kind,
				ApiVersion: apiVersion,
				Url:        url,
			})
		}
	}
	return resources, nil
}

func getCustomResourceDefinitions() ([]Resource, error) {
	indexUrl := "https://raw.githubusercontent.com/datreeio/CRDs-catalog/refs/heads/main/index.yaml"
	var index map[string][]struct {
		Kind       string `yaml:"kind"`
		ApiVersion string `yaml:"apiVersion"`
		Filename   string `yaml:"filename"`
	}
	if err := getYaml(indexUrl, &index); err != nil {
		return nil, fmt.Errorf("get index: %v", err)
	}
	var allCrds []Resource
	for _, crds := range index {
		for _, crd := range crds {
			allCrds = append(allCrds, Resource{
				Kind:       crd.Kind,
				ApiVersion: crd.ApiVersion,
				Url:        fmt.Sprintf("https://raw.githubusercontent.com/datreeio/CRDs-catalog/refs/heads/main/%s", crd.Filename),
			})
		}
	}
	return allCrds, nil
}

func buildKey(kind, apiVersion string) string {
	return fmt.Sprintf("%s_%s", apiVersion, kind)
}

func usMapleKey(key string) (string, string) {
	split := strings.Split(key, "_")
	if len(split) != 2 {
		panic(fmt.Sprintf("expected the key to look like `<apiVersion>_<key>`, got %s", key))
	}
	apiVersion, kind := split[0], split[1]
	return kind, apiVersion
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

func getYaml(url string, output any) error {
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
	if err := yaml.Unmarshal(body, output); err != nil {
		return fmt.Errorf("unmarshal body: %v", err)
	}
	return nil
}
