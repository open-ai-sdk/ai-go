package aikit

// RuntimeContext is shared data supplied to every tool invocation and
// prepare-step callback.
type RuntimeContext map[string]any

// ToolsContext supplies per-tool context values keyed by tool name.
type ToolsContext map[string]any
