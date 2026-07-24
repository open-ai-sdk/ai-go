package ai

import "context"

// RuntimeContext is shared data supplied to every tool invocation and prepare-step callback.
type RuntimeContext map[string]any

// ToolsContext supplies per-tool context values keyed by tool name.
type ToolsContext map[string]any

type toolContextKey struct{}
type runtimeContextKey struct{}

func withToolContexts(ctx context.Context, tool any, runtime RuntimeContext) context.Context {
	ctx = context.WithValue(ctx, toolContextKey{}, tool)
	return context.WithValue(ctx, runtimeContextKey{}, runtime)
}

func ToolContextFrom(ctx context.Context) (any, bool) {
	value, ok := ctx.Value(toolContextKey{}).(any)
	return value, ok
}
func RuntimeContextFrom(ctx context.Context) RuntimeContext {
	value, _ := ctx.Value(runtimeContextKey{}).(RuntimeContext)
	return value
}
func TypedToolContext[C any](ctx context.Context) (C, bool) {
	value, ok := ToolContextFrom(ctx)
	typed, ok := value.(C)
	return typed, ok
}
