package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// New creates a typed tool. In must be a struct or pointer to a struct made
// only from the JSON-schema types documented by schemaForType.
//
// Out is JSON-marshaled by the wrapper. Handler errors are classified as
// execution errors unless they are already one of this package's typed tool
// errors.
func New[In, Out any](
	name, description string,
	fn func(context.Context, In) (Out, error),
) (*Tool, error) {
	if fn == nil {
		return nil, fmt.Errorf("tool.New %q: nil handler", name)
	}

	schema, err := Schema[In]()
	if err != nil {
		return nil, fmt.Errorf("tool.New %q: %w", name, err)
	}

	return &Tool{
		definition: aikit.ToolDefinition{
			Name:        name,
			Description: description,
			InputSchema: schema,
		},
		invokeResult: func(ctx context.Context, raw json.RawMessage) (ExecutionResult, error) {
			var input In
			if err := json.Unmarshal(raw, &input); err != nil {
				return ExecutionResult{}, &InputError{
					ToolName: name,
					Input:    append(json.RawMessage(nil), raw...),
					Cause:    err,
				}
			}

			output, err := fn(ctx, input)
			if err != nil {
				return ExecutionResult{}, classifyHandlerError(name, err)
			}
			return resultFromTypedOutput(name, output)
		},
	}, nil
}

// NewDynamic creates a tool whose schema is known only at runtime. The
// handler receives and returns the raw output bytes used by the agent loop.
func NewDynamic(
	name, description string,
	inputSchema map[string]any,
	fn func(context.Context, json.RawMessage) (json.RawMessage, error),
) (*Tool, error) {
	if fn == nil {
		return nil, fmt.Errorf("tool.NewDynamic %q: nil handler", name)
	}
	return &Tool{
		definition: aikit.ToolDefinition{
			Name:        name,
			Description: description,
			InputSchema: cloneJSONMap(inputSchema),
		},
		invoke: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			output, err := fn(ctx, input)
			if err != nil {
				return nil, classifyHandlerError(name, err)
			}
			return append(json.RawMessage(nil), output...), nil
		},
	}, nil
}

// NewDynamicResult creates a dynamic tool that returns canonical rich output.
func NewDynamicResult(
	name, description string,
	inputSchema map[string]any,
	fn func(context.Context, json.RawMessage) (ExecutionResult, error),
) (*Tool, error) {
	if fn == nil {
		return nil, fmt.Errorf("tool.NewDynamicResult %q: nil handler", name)
	}
	handler := func(ctx context.Context, input json.RawMessage) (ExecutionResult, error) {
		result, err := fn(ctx, append(json.RawMessage(nil), input...))
		if err != nil {
			return ExecutionResult{}, classifyHandlerError(name, err)
		}
		return result.Clone(), nil
	}
	return &Tool{
		definition: aikit.ToolDefinition{
			Name: name, Description: description, InputSchema: cloneJSONMap(inputSchema),
		},
		invokeResult: handler,
		invoke: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			result, err := handler(ctx, input)
			if err != nil {
				return nil, err
			}
			return json.RawMessage(result.Output.ModelText()), nil
		},
	}, nil
}

func resultFromTypedOutput[Out any](name string, value Out) (ExecutionResult, error) {
	if output, ok := any(value).(Output); ok {
		return ExecutionResult{Output: output.Clone()}, nil
	}
	if result, ok := any(value).(ExecutionResult); ok {
		return result.Clone(), nil
	}
	if text, ok := any(value).(string); ok {
		return ExecutionResult{Output: Text(text)}, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ExecutionResult{}, &ExecutionError{ToolName: name, Cause: fmt.Errorf("marshal output: %w", err)}
	}
	output, err := JSON(raw)
	if err != nil {
		return ExecutionResult{}, &ExecutionError{ToolName: name, Cause: err}
	}
	return ExecutionResult{Output: output}, nil
}

func classifyHandlerError(toolName string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrInput) ||
		errors.Is(err, ErrExecution) ||
		errors.Is(err, ErrDenied) ||
		errors.Is(err, ErrNoSuchTool) {
		return err
	}
	return &ExecutionError{ToolName: toolName, Cause: err}
}
