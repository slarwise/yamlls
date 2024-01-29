package schemas

import (
	"fmt"
	"path"
	"strings"
)

func NewKindApiVersionStore(cacheDir string) KindApiVersionStore {
	return KindApiVersionStore{
		CacheDir: cacheDir,
		cache:    map[string]Schema{},
	}
}

type KindApiVersionStore struct {
	CacheDir string
	cache    map[string]Schema
}

func (s *KindApiVersionStore) GetSchema(kind string, apiVersion string) ([]byte, error) {
	key := kindApiVersionKey(kind, apiVersion)
	schema, found := s.cache[key]
	if found {
		return schema.Schema, nil
	}
	URL := buildKindApiVersionURL(kind, apiVersion)
	data, err := callTheInternet(URL)
	if err != nil {
		return nil, err
	}
	filename := path.Join(s.CacheDir, key+".json")
	s.cache[key] = Schema{
		Schema:   data,
		URL:      URL,
		Filename: filename,
	}
	return data, nil
}

// Should this function verify that the URL exists?
func (s *KindApiVersionStore) GetSchemaURL(kind string, apiVersion string) string {
	return buildKindApiVersionURL(kind, apiVersion)
}

func kindApiVersionKey(kind string, apiVersion string) string {
	return fmt.Sprintf("%s-%s", strings.ToLower(kind), strings.ReplaceAll(apiVersion, "/", "-"))
}

func buildKindApiVersionURL(kind string, apiVersion string) string {
	if isCRD(apiVersion) {
		return buildCRDURL(kind, apiVersion)
	}
	return buildKubernetesURL(kind, apiVersion)
}

func buildCRDURL(kind string, apiVersion string) string {
	kind = strings.ToLower(kind)
	splitApiVersion := strings.Split(apiVersion, "/")
	if len(splitApiVersion) != 2 {
		return ""
	}
	host := splitApiVersion[0]
	version := splitApiVersion[1]
	return fmt.Sprintf("https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/%s/%s_%s.json", host, kind, version)
}

func buildKubernetesURL(kind string, apiVersion string) string {
	apiVersion = strings.ReplaceAll(apiVersion, "/", "-")
	kind = strings.ToLower(kind)
	filename := fmt.Sprintf("%s-%s.json", kind, apiVersion)
	return fmt.Sprintf("https://raw.githubusercontent.com/yannh/kubernetes-json-schema/master/master-standalone-strict/%s", filename)
}

func isCRD(apiVersion string) bool {
	return strings.Contains(apiVersion, ".")
}
