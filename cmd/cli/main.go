package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/slarwise/yamlls/pkg/schemas"
	"github.com/tidwall/gjson"
)

// TODO:
//   - Update an existing document and using a path. E.g. I'm in the middle of writing the document and I just want to fill a specific field
//   - When verifying a kustomization.yaml file, ensure that no resources of the same kind have the same name
func main() {
	log.SetFlags(0)
	var schemaPath, path, kind, apiVersion string
	var listDb bool
	flag.StringVar(&schemaPath, "schema", "", "A url (starting with http) or a file path to a `json schema`")
	flag.StringVar(&path, "path", "", "A `path` to a field, e.g. `spec.template`")
	flag.StringVar(&kind, "kind", "", "The `kind` of a kubernetes manifest, e.g. `Deployment`")
	flag.StringVar(&apiVersion, "apiVersion", "", "The `apiVersion` of a kubernetes manifest, e.g. `apps/v1`")
	flag.BoolVar(&listDb, "list", false, "List the available kubernetes manifests")
	flag.Parse()

	if listDb {
		schemas.PrintSchemas()
		os.Exit(0)
	}

	k8s := true
	var jsonSchema map[string]any
	if kind == "" && apiVersion == "" {
		if schemaPath == "" {
			log.Fatalf("-schema must be set")
		}
		jsonSchema = mustLoadJsonSchema(schemaPath)
		k8s = false
	} else if kind != "" {
		if apiVersion == "" {
			apiVersions := schemas.GetApiVersions(kind)
			if len(apiVersions) == 0 {
				log.Fatalf("no apiVersions found for kind `%s`", kind)
			} else if len(apiVersions) == 1 {
				apiVersion = apiVersions[0]
				jsonSchema = mustLoadJsonSchemaFromKindAndApiVersion(kind, apiVersion)
			} else {
				var choice int
				for {
					log.Println("Pick one:")
					for i, apiVersion := range apiVersions {
						log.Printf("%d: %s", i, apiVersion)
					}
					_, err := fmt.Scanln(&choice)
					if err != nil {
						log.Print("try again")
						continue
					}
					if choice >= len(apiVersions) {
						log.Print("too big")
					} else {
						break
					}
				}
				apiVersion = apiVersions[choice]
				jsonSchema = mustLoadJsonSchemaFromKindAndApiVersion(kind, apiVersion)
			}
		} else {
			jsonSchema = mustLoadJsonSchemaFromKindAndApiVersion(kind, apiVersion)
		}
	} else if apiVersion != "" {
		log.Fatalf("must set -kind if setting -apiVersion")
	}
	document, err := schemas.FillFromSchema(jsonSchema)
	if err != nil {
		log.Fatalf("fill from schema: %v", err)
	}
	if k8s {
		k8sDocument := document.(map[string]any)
		k8sDocument["kind"] = kind
		k8sDocument["apiVersion"] = apiVersion
		delete(k8sDocument, "status")
		mustPrint(mustGetPath(k8sDocument, path))
	} else {
		mustPrint(mustGetPath(document, path))
	}
}

func mustMarshalJson(data any) []byte {
	bytes, err := json.Marshal(data)
	if err != nil {
		panic(fmt.Sprintf("expected a valid object at this point when marshalling to json: %v", err))
	}
	return bytes
}

func mustLoadJsonSchema(schemaPath string) map[string]any {
	if !strings.HasPrefix(schemaPath, "http") {
		dir, err := os.Getwd()
		if err != nil {
			panic(fmt.Sprintf("could not find working directory: %v", err))
		}
		schemaPath = "file://" + filepath.Join(dir, schemaPath)
	}
	schema, err := schemas.LoadSchema(schemaPath)
	if err != nil {
		log.Fatalf("load schema: %v", err)
	}
	return schema
}

func mustLoadJsonSchemaFromKindAndApiVersion(kind, apiVersion string) map[string]any {
	url, err := schemas.GetKubernetesSchemaUrl(kind, apiVersion)
	if err != nil {
		log.Fatalf("get url for kind `%s` and apiVersion `%s`: %v", kind, apiVersion, err)
	}
	return mustLoadJsonSchema(url)
}

func mustGetPath(document any, path string) any {
	if path == "" {
		return document
	}
	res := gjson.GetBytes(mustMarshalJson(document), path)
	if !res.Exists() {
		log.Fatalf("path `%s` not found", path)
	}
	return res.Value()
}

func mustPrint(document any) {
	output, err := yaml.MarshalWithOptions(document, yaml.IndentSequence(true))
	if err != nil {
		log.Fatalf("marshal document: %v", err)
	}
	fmt.Printf("%s", output)
}
