package schemas

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/slarwise/yamlls/pkg/parser"
	"github.com/tidwall/gjson"
	"github.com/xeipuuv/gojsonschema"
)

// uri is either a http/https url or an absolute file path like file://
func LoadSchema(uri string) (map[string]any, error) {
	loader := gojsonschema.NewReferenceLoader(uri)
	if _, err := gojsonschema.NewSchemaLoader().Compile(loader); err != nil {
		return nil, fmt.Errorf("compile schema: %v", err)
	}
	jsonSchema_, err := loader.LoadJSON()
	if err != nil {
		return nil, fmt.Errorf("load schema: %v", err)
	}
	jsonSchema, ok := jsonSchema_.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected schema to have type map[string]any")
	}
	return jsonSchema, nil
}

// What should the output be? Should it include the line numbers and columns?
func ValidateJson(schema map[string]any, document []byte) ([]gojsonschema.ResultError, error) {
	result, err := gojsonschema.Validate(gojsonschema.NewGoLoader(schema), gojsonschema.NewBytesLoader(document))
	result.Errors()
	if err != nil {
		return nil, fmt.Errorf("validate against schema: %v", err)
	}
	if result.Valid() {
		return nil, nil
	}
	return result.Errors(), nil
}

type YamlError struct {
	Line, StartCol, EndCol int
	Description            string
	Type                   string
}

var trailingIndex = regexp.MustCompile(`\.\d+$`)

func ValidateYaml(schema map[string]any, document []byte) ([]YamlError, error) {
	bytes, err := yaml.YAMLToJSON(document)
	if err != nil {
		return nil, fmt.Errorf("convert yaml to json: %v", err)
	}
	errors, err := ValidateJson(schema, bytes)
	if err != nil {
		return nil, err
	}
	pathToPosition, err := parser.PathsToPositions(document)
	if err != nil {
		return nil, fmt.Errorf("compute yaml paths to positions: %v", err)
	}
	var yamlErrors []YamlError
	for _, e := range errors {
		field := e.Field()
		var pos parser.Position
		if field == "(root)" {
			pos.Line = 0
			pos.StartCol = 0
			pos.EndCol = 0
		} else {
			if e.Type() == "additional_property_not_allowed" {
				property, hasProperty := e.Details()["property"]
				if hasProperty {
					field = field + "." + property.(string)
				}
			}
			// Turn spec.ports.0 into spec.ports, needed for arrays with required properties
			field = trailingIndex.ReplaceAllString(field, "")

			var found bool
			pos, found = pathToPosition[field]
			if !found {
				return nil, fmt.Errorf("could not find position for error at `%s`", field)
			}
		}
		yamlErrors = append(yamlErrors, YamlError{
			Line:        pos.Line,
			StartCol:    pos.StartCol,
			EndCol:      pos.EndCol,
			Description: e.Description(),
			Type:        e.Type(),
		})
	}
	return yamlErrors, nil
}

var indexPattern = regexp.MustCompile(`.properties.\d+\.`)

// path examples:
// - spec
// - spec.ports
// - spec.ports.3.name
func GetDescription(schema []byte, path string) string {
	path = strings.ReplaceAll(path, ".", ".properties.")
	path = "properties." + path
	path += ".description"
	path = indexPattern.ReplaceAllString(path, ".items.")
	res := gjson.GetBytes(schema, path)
	if !res.Exists() {
		return ""
	}
	return res.String()
}
