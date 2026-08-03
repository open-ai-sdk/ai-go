package ai

import (
	"context"

	"github.com/open-ai-sdk/ai-go/agent/generate"
)

// CompletionObjectResult is a typed direct completion and its normalized
// provider response. Response is present even when decoding fails, allowing a
// caller to inspect usage, finish metadata, and partial output.
type CompletionObjectResult[T any] = generate.CompletionObjectResult[T]

// CompleteObject makes exactly one direct model call, requests an object schema
// derived from T, and unmarshals the completion text into T. It does not run an
// agent tool loop.
//
// The provider may enforce the requested schema. Applications that require
// local semantic validation should validate the returned Object themselves.
func CompleteObject[T any](
	ctx context.Context,
	model LanguageModel,
	request CompletionRequest,
) (CompletionObjectResult[T], error) {
	return generate.CompleteObject[T](ctx, model, request)
}
