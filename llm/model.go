package llm

import (
	"context"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// Model is the minimal contract implemented by language-model providers.
type Model interface {
	ModelID() string
	Stream(context.Context, Request) (<-chan aikit.StreamEvent, error)
}

// CompletionModel is the optional native single-response capability. Models
// that only implement Model remain fully supported through stream aggregation.
// Implementations should retain their untranslated successful response in
// CompletionResponse.RawResponse.
type CompletionModel interface {
	Model
	Complete(context.Context, Request) (*CompletionResponse, error)
}
