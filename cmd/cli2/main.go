package main

import (
	"fmt"

	"github.com/slarwise/yamlls/pkg/schema2"
)

func main() {
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
