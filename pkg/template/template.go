package template

import (
	"fmt"
)

// Generate an example from a schema using zero values
func FillFromSchema(schema map[string]any) (any, error) {
	type_, found := schema["type"]
	if found {
		switch type_ := type_.(type) {
		case string:
			result, err := fillFromType(type_, schema)
			if err != nil {
				return nil, err
			}
			return result, nil
		case []any:
			var singleType string
			for _, v := range type_ {
				v := v.(string)
				if v == "null" {
					continue
				}
				singleType = v
				break
			}
			if singleType == "" {
				return nil, fmt.Errorf("expected at least one type to be not null when type is an array, got %v", type_)
			}
			result, err := fillFromType(singleType, schema)
			if err != nil {
				return nil, err
			}
			return result, nil
		default:
			return nil, fmt.Errorf("expected type to be a string or an array of strings, got %T", type_)
		}
	}

	const_, found := schema["const"]
	if found {
		return const_, nil
	}

	enum, found := schema["enum"]
	if found {
		return enum.([]any)[0], nil
	}

	oneOf, found := schema["oneOf"]
	if found {
		first := oneOf.([]any)[0]
		result, err := FillFromSchema(first.(map[string]any))
		if err != nil {
			return nil, fmt.Errorf("parse oneOf: %v", err)
		}
		return result, nil
	}

	anyOf, found := schema["anyOf"]
	if found {
		first := anyOf.([]any)[0]
		result, err := FillFromSchema(first.(map[string]any))
		if err != nil {
			return nil, fmt.Errorf("parse anyOf: %v", err)
		}
		return result, nil
	}

	_, found = schema["x-kubernetes-preserve-unknown-fields"]
	if found {
		return map[string]any{}, nil
	}

	return nil, fmt.Errorf("expected schema to have type, enum, const, oneOf, anyOf, x-kubernetes-preserve-unknown-fields set, got %v", schema)

}
func fillFromType(type_ string, schema map[string]any) (any, error) {
	switch type_ {
	case "string":
		return "", nil
	case "integer":
		return 0, nil
	case "object":
		properties, found := schema["properties"]
		if !found {
			return map[string]any{}, nil
		}
		result := map[string]any{}
		for k, v := range properties.(map[string]any) {
			subResult, err := FillFromSchema(v.(map[string]any))
			if err != nil {
				return nil, err
			}
			result[k] = subResult
		}
		return result, nil
	case "boolean":
		return false, nil
	case "array":
		items, found := schema["items"]
		if !found {
			return nil, fmt.Errorf("expected a schema of type array to have items")
		}
		subResult, err := FillFromSchema(items.(map[string]any))
		if err != nil {
			return nil, err
		}
		return []any{subResult}, nil
	default:
		panic(fmt.Sprintf("type `%v` not implemented", type_))
	}
}
