package tool

import (
	"context"

	"github.com/open-ai-sdk/ai-go/aikit"
)

type (
	toolContextKey    struct{}
	runtimeContextKey struct{}
	toolCallIDKey     struct{}
)

// WithToolContext attaches the context configured for the executing tool.
func WithToolContext(ctx context.Context, value any) context.Context {
	return context.WithValue(ctx, toolContextKey{}, value)
}

// WithRuntimeContext attaches the run-wide context shared by every tool.
func WithRuntimeContext(
	ctx context.Context,
	value aikit.RuntimeContext,
) context.Context {
	return context.WithValue(ctx, runtimeContextKey{}, value)
}

// WithToolCallID attaches the engine's tool-call identifier.
func WithToolCallID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, toolCallIDKey{}, id)
}

// ToolContextFrom returns the context value configured for the executing tool.
func ToolContextFrom(ctx context.Context) (any, bool) {
	value := ctx.Value(toolContextKey{})
	return value, value != nil
}

// RuntimeContextFrom returns the run-wide context shared by every tool.
func RuntimeContextFrom(ctx context.Context) aikit.RuntimeContext {
	value, ok := ctx.Value(runtimeContextKey{}).(aikit.RuntimeContext)
	if !ok {
		return nil
	}
	return value
}

// TypedContext returns the executing tool's context value asserted to C.
func TypedContext[C any](ctx context.Context) (C, bool) {
	var zero C
	value, ok := ToolContextFrom(ctx)
	if !ok {
		return zero, false
	}
	typed, ok := value.(C)
	if !ok {
		return zero, false
	}
	return typed, true
}

// ToolCallIDFromContext returns the engine-injected tool-call identifier.
func ToolCallIDFromContext(ctx context.Context) string {
	value, ok := ctx.Value(toolCallIDKey{}).(string)
	if !ok {
		return ""
	}
	return value
}
