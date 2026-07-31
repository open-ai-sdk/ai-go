package ai

import (
	"context"

	"github.com/open-ai-sdk/ai-go/aikit"
)

type (
	RuntimeContext = aikit.RuntimeContext
	ToolsContext   = aikit.ToolsContext
)

type (
	toolContextKey    struct{}
	runtimeContextKey struct{}
)

func withToolContexts(ctx context.Context, tool any, runtime RuntimeContext) context.Context {
	ctx = context.WithValue(ctx, toolContextKey{}, tool)
	return context.WithValue(ctx, runtimeContextKey{}, runtime)
}

// ToolContextFrom returns the context value configured for the executing tool.
// The second result reports whether one was supplied.
func ToolContextFrom(ctx context.Context) (any, bool) {
	value := ctx.Value(toolContextKey{})
	return value, value != nil
}

// RuntimeContextFrom returns the run-wide context shared by every tool.
// It is nil when the caller supplied none.
func RuntimeContextFrom(ctx context.Context) RuntimeContext {
	value, ok := ctx.Value(runtimeContextKey{}).(RuntimeContext)
	if !ok {
		return nil
	}
	return value
}

// TypedToolContext returns the executing tool's context value asserted to C.
// The second result is false when no context was supplied or it is not a C.
func TypedToolContext[C any](ctx context.Context) (C, bool) {
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
