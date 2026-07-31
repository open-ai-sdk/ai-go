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
