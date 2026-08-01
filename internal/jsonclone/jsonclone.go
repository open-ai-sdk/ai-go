// Package jsonclone clones JSON-like values while preserving their concrete Go
// container types. It supports typed and named maps, slices, arrays, and cycles.
package jsonclone

import "reflect"

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
	cloned, ok := cloneValue(reflect.ValueOf(values), make(map[visit]reflect.Value)).
		Interface().(map[string]any)
	if !ok {
		panic("jsonclone: cloned map changed type")
	}
	return cloned
}

// Value clones JSON-like map, slice, array, and interface containers while
// retaining their concrete types.
func Value(value any) any {
	if value == nil {
		return nil
	}
	return cloneValue(reflect.ValueOf(value), make(map[visit]reflect.Value)).Interface()
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
		cloned := cloneValue(value.Elem(), seen)
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
