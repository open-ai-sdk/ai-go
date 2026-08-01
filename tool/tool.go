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

// Executor is the string-based execution seam used by existing agent
// integrations. Set adapts it to Invokable tools.
type Executor interface {
	Execute(ctx context.Context, name, argsJSON string) (string, error)
}

// ResultStream receives partial output from a streaming tool.
type ResultStream interface {
	Write(partial string)
}

// StreamingExecutor extends Executor with partial-result streaming.
type StreamingExecutor interface {
	Executor
	ExecuteStreaming(
		ctx context.Context,
		name, argsJSON string,
		stream ResultStream,
	) (string, error)
}

// Tool is the default implementation returned by New and NewDynamic.
type Tool struct {
	definition aikit.ToolDefinition
	invoke     func(context.Context, json.RawMessage) (json.RawMessage, error)
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
		return nil, &ExecutionError{Cause: errNilTool}
	}
	return t.invoke(ctx, input)
}

var _ Invokable = (*Tool)(nil)
