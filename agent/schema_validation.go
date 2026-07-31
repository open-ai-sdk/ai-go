package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
)

// StructuredOutputError reports JSON that is syntactically valid but does not
// satisfy the requested output schema.
type StructuredOutputError struct {
	Path   string
	Reason string
}

func (e *StructuredOutputError) Error() string {
	if e.Path == "" {
		return "agent: structured output " + e.Reason
	}
	return fmt.Sprintf("agent: structured output at %s %s", e.Path, e.Reason)
}

func validateStructuredOutput(raw json.RawMessage, output *OutputSchema) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return &StructuredOutputError{Path: "$", Reason: "is invalid JSON: " + err.Error()}
	}
	schema := output.Schema
	if schema == nil {
		schema = map[string]any{"type": output.Type}
		if output.Type == "json_object" {
			schema["type"] = "object"
		}
	}
	return validateSchemaValue(value, schema, "$")
}

func validateSchemaValue(value any, schema map[string]any, path string) error {
	if allowed, ok := schema["enum"].([]any); ok {
		matched := false
		for _, candidate := range allowed {
			if reflect.DeepEqual(value, candidate) || fmt.Sprint(value) == fmt.Sprint(candidate) {
				matched = true
				break
			}
		}
		if !matched {
			return &StructuredOutputError{Path: path, Reason: "is not an allowed enum value"}
		}
	}

	typeName, _ := schema["type"].(string)
	switch typeName {
	case "", "any":
		return nil
	case "object", "json_object":
		object, ok := value.(map[string]any)
		if !ok {
			return schemaTypeError(path, "object")
		}
		for _, name := range schemaStrings(schema["required"]) {
			if _, exists := object[name]; !exists {
				return &StructuredOutputError{Path: path, Reason: fmt.Sprintf("is missing required property %q", name)}
			}
		}
		properties, _ := schema["properties"].(map[string]any)
		for name, child := range object {
			childSchema, exists := schemaMap(properties[name])
			if !exists {
				if additional, ok := schema["additionalProperties"].(bool); ok && !additional {
					return &StructuredOutputError{Path: path, Reason: fmt.Sprintf("contains unknown property %q", name)}
				}
				continue
			}
			if err := validateSchemaValue(child, childSchema, path+"."+name); err != nil {
				return err
			}
		}
		return nil
	case "array":
		items, ok := value.([]any)
		if !ok {
			return schemaTypeError(path, "array")
		}
		itemSchema, hasItems := schemaMap(schema["items"])
		if !hasItems {
			return nil
		}
		for index, item := range items {
			if err := validateSchemaValue(item, itemSchema, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
		return nil
	case "string":
		if _, ok := value.(string); !ok {
			return schemaTypeError(path, "string")
		}
		return nil
	case "boolean":
		if _, ok := value.(bool); !ok {
			return schemaTypeError(path, "boolean")
		}
		return nil
	case "integer":
		number, ok := value.(json.Number)
		if !ok || !isInteger(number) {
			return schemaTypeError(path, "integer")
		}
		return nil
	case "number":
		if _, ok := value.(json.Number); !ok {
			return schemaTypeError(path, "number")
		}
		return nil
	case "null":
		if value != nil {
			return schemaTypeError(path, "null")
		}
		return nil
	default:
		return &StructuredOutputError{Path: path, Reason: fmt.Sprintf("uses unsupported schema type %q", typeName)}
	}
}

func schemaMap(value any) (map[string]any, bool) {
	schema, ok := value.(map[string]any)
	return schema, ok
}

func schemaStrings(value any) []string {
	switch values := value.(type) {
	case []string:
		return values
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func schemaTypeError(path, expected string) error {
	return &StructuredOutputError{Path: path, Reason: "must be " + expected}
}

func isInteger(number json.Number) bool {
	var integer big.Int
	_, ok := integer.SetString(string(number), 10)
	return ok
}
