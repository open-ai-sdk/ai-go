package tool

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

var (
	timeType            = reflect.TypeOf(time.Time{})
	jsonUnmarshalerType = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
	textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
)

// Schema returns the JSON Schema derived from In.
func Schema[In any]() (map[string]any, error) {
	return schemaForType(reflect.TypeOf((*In)(nil)).Elem())
}

// schemaForType supports structs composed from strings, booleans, integer and
// floating-point numbers, pointers, slices, arrays, and nested structs. Map,
// interface, function, channel, complex, and unsafe-pointer fields are
// rejected when the tool is constructed.
func schemaForType(input reflect.Type) (map[string]any, error) {
	for input.Kind() == reflect.Pointer {
		input = input.Elem()
	}
	if input.Kind() != reflect.Struct {
		return nil, fmt.Errorf(
			"input type %s is unsupported: want a struct or pointer to struct",
			input,
		)
	}
	if input == timeType || implementsCustomJSONDecoding(input) {
		return nil, fmt.Errorf(
			"input type %s is unsupported: custom JSON encoding is not an object schema",
			input,
		)
	}
	return schemaFromStruct(input, nil, map[reflect.Type]bool{})
}

func schemaFromStruct(
	input reflect.Type,
	path []string,
	active map[reflect.Type]bool,
) (map[string]any, error) {
	if active[input] {
		return nil, fmt.Errorf(
			"input field %q has recursive type %s",
			strings.Join(path, "."),
			input,
		)
	}
	active[input] = true
	defer delete(active, input)

	properties := make(map[string]any)
	required := make([]string, 0, input.NumField())
	for i := range input.NumField() {
		field := input.Field(i)
		if !field.IsExported() {
			if isAnonymousStruct(field) {
				return nil, fmt.Errorf(
					"input field %q is an unsupported anonymous embedded struct",
					strings.Join(appendPath(path, field.Name), "."),
				)
			}
			continue
		}

		jsonKey := jsonFieldName(field)
		if jsonKey == "" {
			continue
		}
		if isPromotedStructField(field) {
			return nil, fmt.Errorf(
				"input field %q is an unsupported anonymous embedded struct; add an explicit json name",
				strings.Join(appendPath(path, field.Name), "."),
			)
		}
		fieldPath := appendPath(path, jsonKey)
		property, err := fieldSchema(field, fieldPath, active)
		if err != nil {
			return nil, err
		}
		properties[jsonKey] = property
		if field.Type.Kind() != reflect.Pointer {
			required = append(required, jsonKey)
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema, nil
}

func jsonFieldName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return ""
	}
	if tag == "" {
		return field.Name
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "-" {
		return ""
	}
	if name == "" {
		return field.Name
	}
	return name
}

func fieldSchema(
	field reflect.StructField,
	path []string,
	active map[reflect.Type]bool,
) (map[string]any, error) {
	fieldType := field.Type
	for fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}

	property, err := goTypeToSchema(fieldType, path, active)
	if err != nil {
		return nil, err
	}
	if description := field.Tag.Get("description"); description != "" {
		property["description"] = description
	}
	if enumTag := field.Tag.Get("enum"); enumTag != "" {
		if fieldType.Kind() != reflect.String {
			return nil, fmt.Errorf(
				"input field %q uses enum on unsupported type %s",
				strings.Join(path, "."),
				fieldType,
			)
		}
		values := strings.Split(enumTag, ",")
		enum := make([]any, len(values))
		for i, value := range values {
			enum[i] = strings.TrimSpace(value)
		}
		property["enum"] = enum
	}
	return property, nil
}

func goTypeToSchema(
	input reflect.Type,
	path []string,
	active map[reflect.Type]bool,
) (map[string]any, error) {
	if input == timeType {
		return map[string]any{"type": "string", "format": "date-time"}, nil
	}
	if implementsCustomJSONDecoding(input) {
		return nil, fmt.Errorf(
			"input field %q has unsupported custom JSON encoding on %s",
			strings.Join(path, "."),
			input,
		)
	}
	switch input.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}, nil
	case reflect.Bool:
		return map[string]any{"type": "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}, nil
	case reflect.Slice, reflect.Array:
		itemType := input.Elem()
		for itemType.Kind() == reflect.Pointer {
			itemType = itemType.Elem()
		}
		items, err := goTypeToSchema(itemType, appendPath(path, "[]"), active)
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "array", "items": items}, nil
	case reflect.Struct:
		return schemaFromStruct(input, path, active)
	default:
		return nil, fmt.Errorf(
			"input field %q has unsupported type %s",
			strings.Join(path, "."),
			input,
		)
	}
}

func implementsCustomJSONDecoding(input reflect.Type) bool {
	if input.Implements(jsonUnmarshalerType) ||
		input.Implements(textUnmarshalerType) {
		return true
	}
	pointer := reflect.PointerTo(input)
	return pointer.Implements(jsonUnmarshalerType) ||
		pointer.Implements(textUnmarshalerType)
}

func isPromotedStructField(field reflect.StructField) bool {
	if !isAnonymousStruct(field) || hasExplicitJSONName(field) {
		return false
	}
	return true
}

func isAnonymousStruct(field reflect.StructField) bool {
	if !field.Anonymous {
		return false
	}
	fieldType := field.Type
	for fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}
	return fieldType.Kind() == reflect.Struct
}

func hasExplicitJSONName(field reflect.StructField) bool {
	tag, ok := field.Tag.Lookup("json")
	if !ok {
		return false
	}
	name, _, _ := strings.Cut(tag, ",")
	return name != "" && name != "-"
}

func appendPath(path []string, part string) []string {
	next := make([]string, len(path)+1)
	copy(next, path)
	next[len(path)] = part
	return next
}
