package schema2

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/xeipuuv/gojsonschema"
)

type Store interface {
	get(string) (schema, bool)
}

func NewKubernetesStore() (KubernetesStore, error) {
	db, err := setupKubernetesDatabase()
	if err != nil {
		return KubernetesStore{}, fmt.Errorf("failed to setup database with kubernetes schemas: %v", err)
	}
	return KubernetesStore{db: db}, nil
}

type KubernetesStore struct {
	db kubernetesDb
}

func (s KubernetesStore) get(contents string) (schema, bool) {
	kind, apiVersion := findKindAndApiVersion(contents)
	key := buildKubernetesKey(kind, apiVersion)
	if schema, found := s.db[key]; found {
		return schema, true
	}
	return schema{}, false
}

var (
	kindPattern       = regexp.MustCompile(`^kind: (.+)$`)
	apiVersionPattern = regexp.MustCompile(`^apiVersion: (.+)$`)
)

// NOTE: If kind or apiVersion is set multiple times, the last one is used
func findKindAndApiVersion(contents string) (string, string) {
	var kind, apiVersion string
	for _, line := range strings.FieldsFunc(contents, func(r rune) bool { return r == '\n' }) {
		kindMatch := kindPattern.FindStringSubmatch(line)
		if kindMatch != nil {
			kind = kindMatch[1]
		}
		apiVersionMatch := apiVersionPattern.FindStringSubmatch(line)
		if apiVersionMatch != nil {
			apiVersion = apiVersionMatch[1]
		}
	}
	kind = strings.ReplaceAll(strings.ReplaceAll(kind, `"`, ""), `'`, "")
	apiVersion = strings.ReplaceAll(strings.ReplaceAll(apiVersion, `"`, ""), `'`, "")
	return kind, apiVersion
}

type kubernetesDb map[string]schema

func setupKubernetesDatabase() (kubernetesDb, error) {
	db := kubernetesDb{}
	nativeResources, err := getNativeResourceDefinitions()
	if err != nil {
		return nil, fmt.Errorf("get native resource definitions: %v", err)
	}
	crds, err := getCustomResourceDefinitions()
	if err != nil {
		return nil, fmt.Errorf("get custom resource definitions: %v", err)
	}

	allResources := append(nativeResources, crds...)
	for _, resource := range allResources {
		key := buildKubernetesKey(resource.Kind, resource.ApiVersion)
		db[key] = schema{loader: gojsonschema.NewReferenceLoader(resource.Url)}
	}
	return db, nil
}

type resource struct{ Kind, ApiVersion, Url string }

type definitionsResponse struct {
	Definitions map[string]definition `json:"definitions"`
}

type definition struct {
	GVK []groupVersionKind `json:"x-kubernetes-group-version-kind,omitempty"`
}

type groupVersionKind struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

func getNativeResourceDefinitions() ([]resource, error) {
	definitionsUrl := "https://raw.githubusercontent.com/yannh/kubernetes-json-schema/master/master-standalone-strict/_definitions.json"
	var resp definitionsResponse
	if err := getJson(definitionsUrl, &resp); err != nil {
		return nil, fmt.Errorf("get definitions in yannh/kubernetes-json-schema: %v", err)
	}
	var resources []resource
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
			resources = append(resources, resource{
				Kind:       kind,
				ApiVersion: apiVersion,
				Url:        url,
			})
		}
	}
	return resources, nil
}

func getCustomResourceDefinitions() ([]resource, error) {
	indexUrl := "https://raw.githubusercontent.com/datreeio/CRDs-catalog/refs/heads/main/index.yaml"
	var index map[string][]struct {
		Kind       string `yaml:"kind"`
		ApiVersion string `yaml:"apiVersion"`
		Filename   string `yaml:"filename"`
	}
	if err := getYaml(indexUrl, &index); err != nil {
		return nil, fmt.Errorf("get index: %v", err)
	}
	var allCrds []resource
	for _, crds := range index {
		for _, crd := range crds {
			allCrds = append(allCrds, resource{
				Kind:       crd.Kind,
				ApiVersion: crd.ApiVersion,
				Url:        fmt.Sprintf("https://raw.githubusercontent.com/datreeio/CRDs-catalog/refs/heads/main/%s", crd.Filename),
			})
		}
	}
	return allCrds, nil
}

func buildKubernetesKey(kind, apiVersion string) string {
	return kind + "_" + apiVersion
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
