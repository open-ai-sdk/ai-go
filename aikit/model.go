package aikit

import "context"

// Model is the temporary shared model contract. Phase 03 moves this interface
// to package llm without changing its method set.
type Model interface {
	ModelID() string
	Stream(context.Context, ModelRequest) (<-chan StreamEvent, error)
}

// ModelRequest is the normalized input passed to [Model.Stream].
type ModelRequest struct {
	Instructions    string
	Messages        []Message
	Tools           []ToolDefinition
	ToolChoice      *ToolChoice
	Output          *OutputSchema
	Settings        CallSettings
	ProviderOptions map[string]any
	ToolsContext    ToolsContext
	RuntimeContext  RuntimeContext
}

// OutputSchema describes the desired output mode for a generation call.
type OutputSchema struct {
	Type   string
	Schema map[string]any
}

// CallSettings controls model behavior per request.
type CallSettings struct {
	Temperature   *float32
	MaxTokens     int
	TopP          *float32
	TopK          *int
	Seed          *int
	StopSequences []string
}

// RuntimeContext is shared data supplied to every tool invocation and
// prepare-step callback.
type RuntimeContext map[string]any

// ToolsContext supplies per-tool context values keyed by tool name.
type ToolsContext map[string]any
