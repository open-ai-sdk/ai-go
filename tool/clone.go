package tool

import (
	"encoding/json"
	"reflect"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func cloneDefinition(definition aikit.ToolDefinition) aikit.ToolDefinition {
	definition.InputSchema = cloneJSONMap(definition.InputSchema)
	definition.ContextSchema = cloneJSONMap(definition.ContextSchema)
	return definition
}

func cloneJSONMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned, ok := cloneJSONValue(input).(map[string]any)
	if !ok {
		panic("tool: cloned JSON map changed type")
	}
	return cloned
}

func cloneJSONValue(input any) any {
	if input == nil {
		return nil
	}
	return cloneJSONAny(input, make(map[cloneVisit]reflect.Value))
}

type cloneVisit struct {
	typ      reflect.Type
	pointer  uintptr
	length   int
	capacity int
}

// This walker remains local because tool schemas also clone pointers, while
// internal/jsonclone intentionally treats pointers as scalar values.
func cloneJSONAny(input any, visited map[cloneVisit]reflect.Value) any {
	if input == nil {
		return nil
	}
	switch value := input.(type) {
	case map[string]any:
		return cloneJSONAnyMap(value, visited)
	case []any:
		return cloneJSONAnySlice(value, visited)
	case json.RawMessage:
		return cloneJSONRawMessage(value, visited)
	case []byte:
		return cloneJSONBytes(value, visited)
	case []string:
		return cloneJSONStrings(value, visited)
	case map[string]string:
		return cloneJSONStringMap(value, visited)
	default:
		return cloneJSONReflect(reflect.ValueOf(input), visited).Interface()
	}
}

func cloneJSONAnyMap(
	input map[string]any,
	visited map[cloneVisit]reflect.Value,
) map[string]any {
	if input == nil {
		return nil
	}
	visit := cloneVisit{typ: reflect.TypeOf(input), pointer: reflect.ValueOf(input).Pointer()}
	if output, ok := visited[visit]; ok {
		result, valid := output.Interface().(map[string]any)
		if !valid {
			panic("tool: cached JSON map changed type")
		}
		return result
	}
	output := make(map[string]any, len(input))
	visited[visit] = reflect.ValueOf(output)
	for name, value := range input {
		output[name] = cloneJSONAny(value, visited)
	}
	return output
}

func cloneJSONAnySlice(input []any, visited map[cloneVisit]reflect.Value) []any {
	if input == nil {
		return nil
	}
	value := reflect.ValueOf(input)
	visit := cloneVisit{
		typ:      value.Type(),
		pointer:  value.Pointer(),
		length:   value.Len(),
		capacity: value.Cap(),
	}
	if output, ok := visited[visit]; ok {
		result, valid := output.Interface().([]any)
		if !valid {
			panic("tool: cached JSON slice changed type")
		}
		return result
	}
	output := make([]any, len(input))
	visited[visit] = reflect.ValueOf(output)
	for index, item := range input {
		output[index] = cloneJSONAny(item, visited)
	}
	return output
}

func cloneJSONRawMessage(
	input json.RawMessage,
	visited map[cloneVisit]reflect.Value,
) json.RawMessage {
	if input == nil {
		return nil
	}
	value := reflect.ValueOf(input)
	visit := cloneVisit{typ: value.Type(), pointer: value.Pointer(), length: value.Len(), capacity: value.Cap()}
	if output, ok := visited[visit]; ok {
		result, valid := output.Interface().(json.RawMessage)
		if !valid {
			panic("tool: cached raw message changed type")
		}
		return result
	}
	output := make(json.RawMessage, len(input))
	visited[visit] = reflect.ValueOf(output)
	copy(output, input)
	return output
}

func cloneJSONBytes(input []byte, visited map[cloneVisit]reflect.Value) []byte {
	if input == nil {
		return nil
	}
	value := reflect.ValueOf(input)
	visit := cloneVisit{typ: value.Type(), pointer: value.Pointer(), length: value.Len(), capacity: value.Cap()}
	if output, ok := visited[visit]; ok {
		result, valid := output.Interface().([]byte)
		if !valid {
			panic("tool: cached JSON bytes changed type")
		}
		return result
	}
	output := make([]byte, len(input))
	visited[visit] = reflect.ValueOf(output)
	copy(output, input)
	return output
}

func cloneJSONStrings(input []string, visited map[cloneVisit]reflect.Value) []string {
	if input == nil {
		return nil
	}
	value := reflect.ValueOf(input)
	visit := cloneVisit{typ: value.Type(), pointer: value.Pointer(), length: value.Len(), capacity: value.Cap()}
	if output, ok := visited[visit]; ok {
		result, valid := output.Interface().([]string)
		if !valid {
			panic("tool: cached JSON strings changed type")
		}
		return result
	}
	output := make([]string, len(input))
	visited[visit] = reflect.ValueOf(output)
	copy(output, input)
	return output
}

func cloneJSONStringMap(
	input map[string]string,
	visited map[cloneVisit]reflect.Value,
) map[string]string {
	if input == nil {
		return nil
	}
	value := reflect.ValueOf(input)
	visit := cloneVisit{typ: value.Type(), pointer: value.Pointer()}
	if output, ok := visited[visit]; ok {
		result, valid := output.Interface().(map[string]string)
		if !valid {
			panic("tool: cached JSON string map changed type")
		}
		return result
	}
	output := make(map[string]string, len(input))
	visited[visit] = reflect.ValueOf(output)
	for name, item := range input {
		output[name] = item
	}
	return output
}

func cloneJSONReflect(
	input reflect.Value,
	visited map[cloneVisit]reflect.Value,
) reflect.Value {
	switch input.Kind() {
	case reflect.Interface:
		if input.IsNil() {
			return reflect.Zero(input.Type())
		}
		output := reflect.New(input.Type()).Elem()
		output.Set(reflect.ValueOf(cloneJSONAny(input.Interface(), visited)))
		return output
	case reflect.Map:
		if input.IsNil() {
			return reflect.Zero(input.Type())
		}
		visit := cloneVisit{typ: input.Type(), pointer: input.Pointer()}
		if output, ok := visited[visit]; ok {
			return output
		}
		output := reflect.MakeMapWithSize(input.Type(), input.Len())
		visited[visit] = output
		iter := input.MapRange()
		for iter.Next() {
			output.SetMapIndex(
				iter.Key(),
				cloneJSONReflect(iter.Value(), visited),
			)
		}
		return output
	case reflect.Pointer:
		if input.IsNil() {
			return reflect.Zero(input.Type())
		}
		visit := cloneVisit{typ: input.Type(), pointer: input.Pointer()}
		if output, ok := visited[visit]; ok {
			return output
		}
		output := reflect.New(input.Type().Elem())
		visited[visit] = output
		output.Elem().Set(cloneJSONReflect(input.Elem(), visited))
		return output
	case reflect.Slice:
		if input.IsNil() {
			return reflect.Zero(input.Type())
		}
		visit := cloneVisit{
			typ:      input.Type(),
			pointer:  input.Pointer(),
			length:   input.Len(),
			capacity: input.Cap(),
		}
		if output, ok := visited[visit]; ok {
			return output
		}
		output := reflect.MakeSlice(input.Type(), input.Len(), input.Len())
		visited[visit] = output
		for i := range input.Len() {
			output.Index(i).Set(cloneJSONReflect(input.Index(i), visited))
		}
		return output
	case reflect.Array:
		output := reflect.New(input.Type()).Elem()
		for i := range input.Len() {
			output.Index(i).Set(cloneJSONReflect(input.Index(i), visited))
		}
		return output
	default:
		return input
	}
}
