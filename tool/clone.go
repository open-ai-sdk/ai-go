package tool

import (
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
	return cloneJSONValue(input).(map[string]any)
}

func cloneJSONValue(input any) any {
	if input == nil {
		return nil
	}
	return cloneJSONReflect(
		reflect.ValueOf(input),
		make(map[cloneVisit]reflect.Value),
	).Interface()
}

type cloneVisit struct {
	typ      reflect.Type
	pointer  uintptr
	length   int
	capacity int
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
		output.Set(cloneJSONReflect(input.Elem(), visited))
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
