package ai

import (
	"context"

	"github.com/open-ai-sdk/ai-go/llm"
)

type (
	EmbedRequest     = llm.EmbedRequest
	EmbedManyRequest = llm.EmbedManyRequest
	EmbedResult      = llm.EmbedResult
	EmbedManyResult  = llm.EmbedManyResult
)

// Embed generates an embedding vector for a single text.
func Embed(ctx context.Context, req EmbedRequest) (EmbedResult, error) {
	return llm.Embed(ctx, req)
}

// EmbedMany generates embedding vectors for multiple texts in a single batch call.
// Results are returned in the same order as the input texts.
func EmbedMany(ctx context.Context, req EmbedManyRequest) (EmbedManyResult, error) {
	return llm.EmbedMany(ctx, req)
}
