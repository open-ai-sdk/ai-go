package generate

import (
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

// LanguageModel is the shared model contract. The alias keeps existing ai
// consumers source-compatible while the engine consumes the same type directly.
type LanguageModel = llm.Model

// EmbeddingModel is the interface a provider must implement for text embeddings.
type EmbeddingModel = llm.EmbeddingModel

// LanguageModelRequest is the normalized input passed to LanguageModel.Stream.
type LanguageModelRequest = llm.Request

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
