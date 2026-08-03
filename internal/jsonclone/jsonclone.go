// Package jsonclone clones JSON-like values while preserving their concrete Go
// container types. It supports typed and named maps, slices, arrays, and cycles.
package jsonclone

import (
	"encoding/json"
	"reflect"
)

type visit struct {
	kind     reflect.Kind
	typ      reflect.Type
	pointer  uintptr
	length   int
	capacity int
}

// Map clones a map and every JSON-like container reachable from its values.
// Scalar values, functions, channels, structs, and pointers are copied as-is.
func Map(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	return cloneStringAnyMap(values, make(map[visit]reflect.Value))
}

// Value clones JSON-like map, slice, array, and interface containers while
// retaining their concrete types.
func Value(value any) any {
	if value == nil {
		return nil
	}
	return cloneAny(value, make(map[visit]reflect.Value))
}

func cloneAny(value any, seen map[visit]reflect.Value) any {
	if value == nil {
		return nil
	}
	switch input := value.(type) {
	case map[string]any:
		return cloneStringAnyMap(input, seen)
	case []any:
		return cloneAnySlice(input, seen)
	case json.RawMessage:
		return cloneRawMessage(input, seen)
	case []byte:
		return cloneBytes(input, seen)
	case []string:
		return cloneStrings(input, seen)
	case map[string]string:
		return cloneStringMap(input, seen)
	default:
		return cloneValue(reflect.ValueOf(value), seen).Interface()
	}
}

func cloneStringAnyMap(
	input map[string]any,
	seen map[visit]reflect.Value,
) map[string]any {
	if input == nil {
		return nil
	}
	key := identity(reflect.ValueOf(input))
	if cloned, ok := seen[key]; ok {
		result, valid := cloned.Interface().(map[string]any)
		if !valid {
			panic("jsonclone: cached map changed type")
		}
		return result
	}
	result := make(map[string]any, len(input))
	seen[key] = reflect.ValueOf(result)
	for name, value := range input {
		result[name] = cloneAny(value, seen)
	}
	return result
}

func cloneAnySlice(input []any, seen map[visit]reflect.Value) []any {
	if input == nil {
		return nil
	}
	key := identity(reflect.ValueOf(input))
	if cloned, ok := seen[key]; ok {
		result, valid := cloned.Interface().([]any)
		if !valid {
			panic("jsonclone: cached slice changed type")
		}
		return result
	}
	result := make([]any, len(input))
	seen[key] = reflect.ValueOf(result)
	for index, value := range input {
		result[index] = cloneAny(value, seen)
	}
	return result
}

func cloneRawMessage(input json.RawMessage, seen map[visit]reflect.Value) json.RawMessage {
	if input == nil {
		return nil
	}
	key := identity(reflect.ValueOf(input))
	if cloned, ok := seen[key]; ok {
		result, valid := cloned.Interface().(json.RawMessage)
		if !valid {
			panic("jsonclone: cached raw message changed type")
		}
		return result
	}
	result := make(json.RawMessage, len(input))
	seen[key] = reflect.ValueOf(result)
	copy(result, input)
	return result
}

func cloneBytes(input []byte, seen map[visit]reflect.Value) []byte {
	if input == nil {
		return nil
	}
	key := identity(reflect.ValueOf(input))
	if cloned, ok := seen[key]; ok {
		result, valid := cloned.Interface().([]byte)
		if !valid {
			panic("jsonclone: cached bytes changed type")
		}
		return result
	}
	result := make([]byte, len(input))
	seen[key] = reflect.ValueOf(result)
	copy(result, input)
	return result
}

func cloneStrings(input []string, seen map[visit]reflect.Value) []string {
	if input == nil {
		return nil
	}
	key := identity(reflect.ValueOf(input))
	if cloned, ok := seen[key]; ok {
		result, valid := cloned.Interface().([]string)
		if !valid {
			panic("jsonclone: cached strings changed type")
		}
		return result
	}
	result := make([]string, len(input))
	seen[key] = reflect.ValueOf(result)
	copy(result, input)
	return result
}

func cloneStringMap(input map[string]string, seen map[visit]reflect.Value) map[string]string {
	if input == nil {
		return nil
	}
	key := identity(reflect.ValueOf(input))
	if cloned, ok := seen[key]; ok {
		result, valid := cloned.Interface().(map[string]string)
		if !valid {
			panic("jsonclone: cached string map changed type")
		}
		return result
	}
	result := make(map[string]string, len(input))
	seen[key] = reflect.ValueOf(result)
	for name, value := range input {
		result[name] = value
	}
	return result
}

func cloneValue(value reflect.Value, seen map[visit]reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}

	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.ValueOf(cloneAny(value.Interface(), seen))
		result := reflect.New(value.Type()).Elem()
		result.Set(cloned)
		return result

	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		key := identity(value)
		if cloned, ok := seen[key]; ok {
			return cloned
		}
		result := reflect.MakeMapWithSize(value.Type(), value.Len())
		seen[key] = result
		iterator := value.MapRange()
		for iterator.Next() {
			result.SetMapIndex(
				iterator.Key(),
				cloneValue(iterator.Value(), seen),
			)
		}
		return result

	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		key := identity(value)
		if cloned, ok := seen[key]; ok {
			return cloned
		}
		result := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		seen[key] = result
		for i := 0; i < value.Len(); i++ {
			result.Index(i).Set(cloneValue(value.Index(i), seen))
		}
		return result

	case reflect.Array:
		result := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			result.Index(i).Set(cloneValue(value.Index(i), seen))
		}
		return result

	default:
		return value
	}
}

func identity(value reflect.Value) visit {
	key := visit{
		kind:    value.Kind(),
		typ:     value.Type(),
		pointer: uintptr(value.UnsafePointer()),
	}
	if value.Kind() == reflect.Slice {
		key.length = value.Len()
		key.capacity = value.Cap()
	}
	return key
}
