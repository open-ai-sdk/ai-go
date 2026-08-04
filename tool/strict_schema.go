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
		return strictStructSchema(input, path, active)
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

func strictStructSchema(input reflect.Type, path []string, active map[reflect.Type]bool) (map[string]any, error) {
	if active[input] {
		return nil, fmt.Errorf("output field %q has recursive type %s", strings.Join(path, "."), input)
	}
	active[input] = true
	defer delete(active, input)

	properties := make(map[string]any)
	required := make([]string, 0, input.NumField())
	for i := range input.NumField() {
		name, property, include, err := strictSchemaField(input.Field(i), path, active)
		if err != nil {
			return nil, err
		}
		if !include {
			continue
		}
		properties[name] = property
		required = append(required, name)
	}
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}, nil
}

func strictSchemaField(
	field reflect.StructField,
	path []string,
	active map[reflect.Type]bool,
) (string, map[string]any, bool, error) {
	if !field.IsExported() {
		if isAnonymousStruct(field) {
			return "", nil, false, fmt.Errorf(
				"output field %q is an unsupported anonymous embedded struct",
				strings.Join(appendPath(path, field.Name), "."),
			)
		}
		return "", nil, false, nil
	}
	name := jsonFieldName(field)
	if name == "" {
		return "", nil, false, nil
	}
	if isPromotedStructField(field) {
		return "", nil, false, fmt.Errorf(
			"output field %q is an unsupported anonymous embedded struct; add an explicit json name",
			strings.Join(appendPath(path, field.Name), "."),
		)
	}
	property, err := strictSchemaValue(field.Type, appendPath(path, name), active)
	if err != nil {
		return "", nil, false, err
	}
	optional := field.Type.Kind() == reflect.Pointer || jsonTagHasOption(field, "omitempty")
	if optional {
		makeNullable(property)
	}
	if description := field.Tag.Get("description"); description != "" {
		property["description"] = description
	}
	if err := addStrictEnum(property, field, appendPath(path, name), optional); err != nil {
		return "", nil, false, err
	}
	return name, property, true, nil
}

func addStrictEnum(property map[string]any, field reflect.StructField, path []string, optional bool) error {
	enumTag := field.Tag.Get("enum")
	if enumTag == "" {
		return nil
	}
	if baseType(field.Type).Kind() != reflect.String {
		return fmt.Errorf("output field %q uses enum on unsupported type %s", strings.Join(path, "."), field.Type)
	}
	values := make([]any, 0, len(strings.Split(enumTag, ","))+1)
	for _, value := range strings.Split(enumTag, ",") {
		values = append(values, strings.TrimSpace(value))
	}
	if optional {
		values = append(values, nil)
	}
	property["enum"] = values
	return nil
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
