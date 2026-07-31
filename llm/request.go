package llm

import "github.com/open-ai-sdk/ai-go/aikit"

// Request is the normalized input passed to [Model.Stream].
type Request struct {
	Instructions    string
	Messages        []aikit.Message
	Tools           []aikit.ToolDefinition
	ToolChoice      *aikit.ToolChoice
	Output          *OutputSchema
	Settings        CallSettings
	ProviderOptions map[string]any
	ToolsContext    aikit.ToolsContext
	RuntimeContext  aikit.RuntimeContext
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
