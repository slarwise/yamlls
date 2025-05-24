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
	"github.com/slarwise/yamlls/pkg/template"
	"github.com/tidwall/gjson"
	"github.com/xeipuuv/gojsonschema"
)

// TODO:
//   - Pick a schema based on kind and apiVersion
//   - Pick a schema based on kind (interactive)
//   - Pick a schema based on apiVersion (interactive)
//   - Pick a schema based on group (interactive)
//   - Update an existing document and using a path. E.g. I'm in the middle of writing the document and I just want to fill a specific field
func main() {
	log.SetFlags(0)
	var schemaPath string
	flag.StringVar(&schemaPath, "schema", "", "A url (starting with http) or a file path to a `json schema`")
	var path string
	flag.StringVar(&path, "path", "", "A `path` to a field, e.g. `spec.template`")
	flag.Parse()
	if schemaPath == "" {
		log.Fatalf("-schema must be set")
	}

	jsonSchema := mustLoadJsonSchema(schemaPath)
	jsonSchema = mustGetSubSchema(jsonSchema, path)
	document, err := template.FillFromSchema(jsonSchema)
	if err != nil {
		log.Fatalf("fill schema: %v", err)
	}
	output, err := yaml.MarshalWithOptions(document, yaml.IndentSequence(true))
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

	loader := gojsonschema.NewReferenceLoader(schemaPath)
	if _, err := gojsonschema.NewSchemaLoader().Compile(loader); err != nil {
		log.Fatalf("invalid json schema: %v", err)
	}
	jsonSchema_, err := loader.LoadJSON()
	if err != nil {
		panic(fmt.Sprintf("the json schema is invalid json even though it was validated. This should not happen. Got %v", err))
	}
	jsonSchema, ok := jsonSchema_.(map[string]any)
	if !ok {
		panic("expected the json schema to be of type map[string]any")
	}
	return jsonSchema
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
