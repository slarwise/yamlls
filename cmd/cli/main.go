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
//   - Pick a schema based on apiVersion (interactive)
//   - Pick a schema based on group (interactive)
//   - Update an existing document and using a path. E.g. I'm in the middle of writing the document and I just want to fill a specific field
//   - Maybe always render the whole schema and then pick out the needed bit from the path. That makes it easier to always set kind and
//     apiVersion and remove status
//   - Don't lowercase kind in the database, need to keep the casing so we can put it into the output document
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
	if kind != "" && apiVersion == "" {
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
	} else if kind != "" || apiVersion != "" {
		if kind == "" || apiVersion == "" {
			log.Fatalf("both -kind and -apiVersion must be set")
		}
		jsonSchema = mustLoadJsonSchemaFromKindAndApiVersion(kind, apiVersion)
	} else {
		k8s = false
		if schemaPath == "" {
			log.Fatalf("-schema must be set")
		}
		jsonSchema = mustLoadJsonSchema(schemaPath)
	}
	jsonSchema = mustGetSubSchema(jsonSchema, path)
	document, err := schemas.FillFromSchema(jsonSchema)
	if err != nil {
		log.Fatalf("fill schema: %v", err)
	}
	var document2 map[string]any
	if k8s {
		document2 = document.(map[string]any)
		document2["kind"] = kind
		document2["apiVersion"] = apiVersion
		delete(document2, "status")
	} else {
		document2 = document.(map[string]any)
	}
	output, err := yaml.MarshalWithOptions(document2, yaml.IndentSequence(true))
	if err != nil {
		log.Fatalf("marshal document: %v", err)
	}
	fmt.Printf("%s", output)
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

// Support only path to properties, e.g. a.b.c, and c must be an object
func mustGetSubSchema(schema map[string]any, path string) map[string]any {
	if path == "" {
		return schema
	}
	path = "properties." + strings.ReplaceAll(path, ".", ".properties.")
	bytes, err := json.Marshal(schema)
	if err != nil {
		log.Fatalf("expected schema to be valid json, got %v", err)
	}
	result := gjson.GetBytes(bytes, path)
	switch value := result.Value().(type) {
	case map[string]any:
		return value
	default:
		log.Fatalf("no subschema found on path %s", path)
		return nil // hmm, this is unreachable
	}
}

func mustLoadJsonSchemaFromKindAndApiVersion(kind, apiVersion string) map[string]any {
	// TODO: Support CRDs
	url, err := schemas.GetKubernetesSchemaUrl(kind, apiVersion)
	if err != nil {
		log.Fatalf("get url for kind `%s` and apiVersion `%s`: %v", kind, apiVersion, err)
	}
	return mustLoadJsonSchema(url)
}
