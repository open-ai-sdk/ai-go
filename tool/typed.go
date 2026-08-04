package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
)

// Typed is the Go-native interface for authoring a typed tool as a value.
//
// Describe supplies the provider-facing name, description, and JSON Schema.
// Call receives decoded Args and returns a typed Output. Adapt erases a Typed
// value to [Invokable] so tools with different argument and output types can
// share one [Set].
//
// This is the struct-based counterpart to [New], which is more concise for a
// single function. Typed tools can retain dependencies and configuration in
// their receiver and can provide a hand-authored schema when needed.
type Typed[Args any, Output any] interface {
	Definition
	Call(context.Context, Args) (Output, error)
}

// Adapt converts a Typed tool into the erased [Invokable] runtime form.
//
// The definition is captured when Adapt is called. Input JSON is decoded into
// Args before Call runs, and Output follows the same text, JSON, rich-output,
// and error behavior as [New]. Register the returned tool with [NewSet].
func Adapt[Args, Output any](typed Typed[Args, Output]) (*Tool, error) {
	if isNilTyped(typed) {
		return nil, fmt.Errorf("tool.Adapt: nil typed tool")
	}

	definition := typed.Describe()
	name := definition.Name
	return &Tool{
		definition: cloneDefinition(definition),
		invokeResult: func(ctx context.Context, raw json.RawMessage) (ExecutionResult, error) {
			var input Args
			if err := json.Unmarshal(raw, &input); err != nil {
				return ExecutionResult{}, &InputError{
					ToolName: name,
					Input:    append(json.RawMessage(nil), raw...),
					Cause:    err,
				}
			}

			output, err := typed.Call(ctx, input)
			if err != nil {
				return ExecutionResult{}, classifyHandlerError(name, err)
			}
			return resultFromTypedOutput(name, output)
		},
	}, nil
}

func isNilTyped(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
