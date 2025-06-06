package main

import (
	"fmt"

	"github.com/slarwise/yamlls/pkg/schema2"
)

func main() {
	// docs()
	hover()
	validate()
}

func docs() {
	schema, found := schema2.GetSchema(`kind: Service
apiVersion: v1`)
	if !found {
		panic("schema not found")
	}
	for _, p := range schema.Docs() {
		fmt.Printf("%s:\n", p.Path)
		fmt.Printf("  %s\n", p.Type)
		fmt.Printf("  %s\n", p.Description)
	}
}

func hover() {
	input := `kind: Service
apiVersion: v1
metadata:
  name: hej
`
	schema, schemaFound := schema2.GetSchema(input)
	if !schemaFound {
		panic("schema not found")
	}
	doc, ok := schema2.NewYamlDocument(input)
	if !ok {
		panic("invalid input")
	}
	paths := doc.Paths()
	path, found := paths.AtCursor(3, 4)
	if !found {
		fmt.Printf("No path found at line 3 and char 4")
	}
	var desc string
	pathFound := false
	for _, p := range schema.Docs() {
		if p.Path == path {
			desc = p.Description
			pathFound = true
			break
		}
	}
	if !pathFound {
		fmt.Printf("could not find path %s\n", path)
	}
	fmt.Printf("description for path `%s`:\n", path)
	fmt.Println(desc)
}

func validate() {
	file := `kind: Service
apiVersion: v1
metadata:
  name: 3
`
	errors := schema2.ValidateFile(file)
	for _, e := range errors {
		fmt.Println(e)
	}
}
