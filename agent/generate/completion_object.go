package generate

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/tool"
)

// CompletionObjectResult is a typed direct completion and its normalized
// provider response. Response is present even when decoding fails, allowing a
// caller to inspect usage, finish metadata, and partial output.
type CompletionObjectResult[T any] struct {
	Object   T
	Response *llm.CompletionResponse
}

// CompleteObject makes exactly one direct model call, requests an object schema
// derived from T, and unmarshals the completion text into T. It does not run an
// agent tool loop.
//
// The provider may enforce the requested schema. Applications that require
// local semantic validation should validate the returned Object themselves.
func CompleteObject[T any](
	ctx context.Context,
	model LanguageModel,
	request llm.CompletionRequest,
) (CompletionObjectResult[T], error) {
	schema, err := tool.Schema[T]()
	if err != nil {
		return CompletionObjectResult[T]{}, fmt.Errorf("ai: CompleteObject: %w", err)
	}

	request.Output = &llm.OutputSchema{Type: "object", Schema: schema}
	response, err := llm.Complete(ctx, model, request)
	result := CompletionObjectResult[T]{Response: response}
	if err != nil {
		return result, err
	}
	if response == nil {
		return result, fmt.Errorf("ai: CompleteObject: model returned no response")
	}
	if err := json.Unmarshal([]byte(response.Text), &result.Object); err != nil {
		return result, fmt.Errorf("ai: CompleteObject: unmarshal result: %w", err)
	}
	return result, nil
}
