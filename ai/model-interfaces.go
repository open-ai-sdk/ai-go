package ai

import (
	"context"

	"github.com/open-ai-sdk/ai-go/aitypes"
)

// LanguageModel is the interface a provider must implement for chat/text generation.
type LanguageModel interface {
	// ModelID returns the provider-specific model identifier.
	ModelID() string

	// Stream starts a streaming chat completion and returns a channel of StreamEvents.
	//
	// Context contract: Stream must stop producing and close its channel when ctx
	// is cancelled, releasing any underlying resources (e.g. the HTTP response
	// body). Every send on the returned channel must select on ctx.Done() so a
	// stalled consumer cannot park the producer. Callers that stop reading before
	// the channel closes must cancel ctx; they are not required to drain.
	Stream(ctx context.Context, req LanguageModelRequest) (<-chan StreamEvent, error)
}

// EmbeddingModel is the interface a provider must implement for text embeddings.
type EmbeddingModel interface {
	// ModelID returns the provider-specific model identifier.
	ModelID() string

	// Embed generates an embedding vector for a single text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch generates embedding vectors for multiple texts.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// LanguageModelRequest is the normalized input passed to LanguageModel.Stream.
// Providers should read ToolChoice to determine the tool selection policy.
type LanguageModelRequest struct {
	// Instructions is the system prompt, already resolved from GenerateTextRequest.Instructions.
	Instructions string
	// Messages is the full conversation history for this step.
	Messages []Message
	// Tools is the list of callable function tools available for this step.
	Tools []ToolDefinition
	// ToolChoice controls which tool the model must call. Nil means auto.
	ToolChoice *ToolChoice
	// Output optionally constrains the output to a JSON schema or mode.
	Output *OutputSchema
	// Settings holds per-request model parameters.
	Settings CallSettings
	// ProviderOptions carries provider-specific options keyed by provider name.
	ProviderOptions map[string]any
}

// Warning, Source, StreamEvent and the StreamEventType enum are aliases of the
// shared aitypes package (see ai/types.go for the full alias set).
type (
	Warning         = aitypes.Warning
	StreamEventType = aitypes.StreamEventType
	Source          = aitypes.Source
	StreamEvent     = aitypes.StreamEvent
)

const (
	StreamEventTextDelta      = aitypes.StreamEventTextDelta
	StreamEventReasoningDelta = aitypes.StreamEventReasoningDelta
	StreamEventToolCallDelta  = aitypes.StreamEventToolCallDelta
	StreamEventUsage          = aitypes.StreamEventUsage
	StreamEventFinish         = aitypes.StreamEventFinish
	StreamEventError          = aitypes.StreamEventError
	StreamEventSource         = aitypes.StreamEventSource
	StreamEventFileDelta      = aitypes.StreamEventFileDelta
)
