package tool

import (
	"context"
	"encoding/json"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// Definition describes a tool to a model.
type Definition interface {
	Describe() aikit.ToolDefinition
}

// Invokable is a described tool that can be called through the erased JSON
// runtime boundary.
type Invokable interface {
	Definition
	Invoke(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}

// Tool is the default implementation returned by New and NewDynamic.
type Tool struct {
	definition   aikit.ToolDefinition
	invoke       func(context.Context, json.RawMessage) (json.RawMessage, error)
	invokeResult func(context.Context, json.RawMessage) (ExecutionResult, error)
}

// Describe returns the provider-facing tool definition.
func (t *Tool) Describe() aikit.ToolDefinition {
	if t == nil {
		return aikit.ToolDefinition{}
	}
	return cloneDefinition(t.definition)
}

// Invoke calls the tool through its erased JSON boundary.
func (t *Tool) Invoke(
	ctx context.Context,
	input json.RawMessage,
) (json.RawMessage, error) {
	if t == nil || t.invoke == nil {
		if t != nil && t.invokeResult != nil {
			result, err := t.invokeResult(ctx, input)
			if err != nil {
				return nil, err
			}
			return result.Output.LegacyJSON(), nil
		}
		return nil, &ExecutionError{Cause: errNilTool}
	}
	return t.invoke(ctx, input)
}

// InvokeResult calls the rich result path. Tools created by the compatibility
// constructors retain their exact legacy Invoke bytes.
func (t *Tool) InvokeResult(ctx context.Context, input json.RawMessage) (ExecutionResult, error) {
	if t == nil {
		return ExecutionResult{}, &ExecutionError{Cause: errNilTool}
	}
	if t.invokeResult != nil {
		return t.invokeResult(ctx, input)
	}
	if t.invoke == nil {
		return ExecutionResult{}, &ExecutionError{Cause: errNilTool}
	}
	raw, err := t.invoke(ctx, input)
	if err != nil {
		return ExecutionResult{}, err
	}
	return ResultFromLegacy(raw), nil
}

var (
	_ Invokable       = (*Tool)(nil)
	_ ResultInvokable = (*Tool)(nil)
)
