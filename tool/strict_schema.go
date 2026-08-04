package tool

import (
	"fmt"
	"reflect"
	"strings"
)

// StrictSchema returns a JSON Schema suitable for providers which require
// strict object schemas. Unlike Schema, every property is required and fields
// that Go may omit are represented as nullable. It is intentionally separate
// from Schema: tool input schemas retain their established wire shape.
func StrictSchema[T any]() (map[string]any, error) {
	return strictSchemaForType(reflect.TypeOf((*T)(nil)).Elem())
}

func strictSchemaForType(input reflect.Type) (map[string]any, error) {
	for input.Kind() == reflect.Pointer {
		input = input.Elem()
	}
	if input == timeType || implementsCustomJSONDecoding(input) {
		return nil, fmt.Errorf("output type %s is unsupported: custom JSON encoding is not a schema", input)
	}
	return strictSchemaValue(input, nil, map[reflect.Type]bool{})
}

func strictSchemaValue(input reflect.Type, path []string, active map[reflect.Type]bool) (map[string]any, error) {
	for input.Kind() == reflect.Pointer {
		input = input.Elem()
	}
	if input.Kind() == reflect.Struct {
		if active[input] {
			return nil, fmt.Errorf("output field %q has recursive type %s", strings.Join(path, "."), input)
		}
		active[input] = true
		defer delete(active, input)

		properties := make(map[string]any)
		required := make([]string, 0, input.NumField())
		for i := range input.NumField() {
			field := input.Field(i)
			if !field.IsExported() {
				if isAnonymousStruct(field) {
					return nil, fmt.Errorf("output field %q is an unsupported anonymous embedded struct", strings.Join(appendPath(path, field.Name), "."))
				}
				continue
			}
			name := jsonFieldName(field)
			if name == "" {
				continue
			}
			if isPromotedStructField(field) {
				return nil, fmt.Errorf("output field %q is an unsupported anonymous embedded struct; add an explicit json name", strings.Join(appendPath(path, field.Name), "."))
			}
			fieldPath := appendPath(path, name)
			property, err := strictSchemaValue(field.Type, fieldPath, active)
			if err != nil {
				return nil, err
			}
			optional := field.Type.Kind() == reflect.Pointer || jsonTagHasOption(field, "omitempty")
			if optional {
				makeNullable(property)
			}
			if description := field.Tag.Get("description"); description != "" {
				property["description"] = description
			}
			if enumTag := field.Tag.Get("enum"); enumTag != "" {
				if baseType(field.Type).Kind() != reflect.String {
					return nil, fmt.Errorf("output field %q uses enum on unsupported type %s", strings.Join(fieldPath, "."), field.Type)
				}
				values := make([]any, 0, len(strings.Split(enumTag, ","))+1)
				for _, value := range strings.Split(enumTag, ",") {
					values = append(values, strings.TrimSpace(value))
				}
				if optional {
					values = append(values, nil)
				}
				property["enum"] = values
			}
			properties[name] = property
			required = append(required, name)
		}
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}, nil
	}
	if input.Kind() == reflect.Slice || input.Kind() == reflect.Array {
		items, err := strictSchemaValue(input.Elem(), appendPath(path, "[]"), active)
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "array", "items": items}, nil
	}

	property, err := goTypeToSchema(input, path, active)
	if err != nil {
		return nil, err
	}
	return property, nil
}

func baseType(input reflect.Type) reflect.Type {
	for input.Kind() == reflect.Pointer {
		input = input.Elem()
	}
	return input
}

func jsonTagHasOption(field reflect.StructField, option string) bool {
	_, options, found := strings.Cut(field.Tag.Get("json"), ",")
	if !found {
		return false
	}
	for _, value := range strings.Split(options, ",") {
		if value == option {
			return true
		}
	}
	return false
}

func makeNullable(schema map[string]any) {
	if schemaType, ok := schema["type"].(string); ok {
		schema["type"] = []any{schemaType, "null"}
	}
}
