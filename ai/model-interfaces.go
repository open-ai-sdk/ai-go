package ai

import (
	"context"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// LanguageModel is the shared model contract. The alias keeps existing ai
// consumers source-compatible while the engine consumes the same type directly.
type LanguageModel = aikit.Model

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
type LanguageModelRequest = aikit.ModelRequest

// Warning, Source, StreamEvent and the StreamEventType enum are aliases of the
// shared aikit package (see ai/types.go for the full alias set).
type (
	Warning         = aikit.Warning
	StreamEventType = aikit.StreamEventType
	Source          = aikit.Source
	StreamEvent     = aikit.StreamEvent
)

const (
	StreamEventTextDelta      = aikit.StreamEventTextDelta
	StreamEventReasoningDelta = aikit.StreamEventReasoningDelta
	StreamEventToolCallDelta  = aikit.StreamEventToolCallDelta
	StreamEventUsage          = aikit.StreamEventUsage
	StreamEventFinish         = aikit.StreamEventFinish
	StreamEventError          = aikit.StreamEventError
	StreamEventSource         = aikit.StreamEventSource
	StreamEventFileDelta      = aikit.StreamEventFileDelta
)
