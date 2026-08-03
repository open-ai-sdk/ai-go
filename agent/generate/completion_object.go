package generate

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/open-ai-sdk/ai-go/agent"
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
		return CompletionObjectResult[T]{}, &agent.StructuredOutputError{
			Kind: agent.StructuredOutputErrorKindPrompt, Reason: "invalid output request", Cause: err,
		}
	}

	request.Output = &llm.OutputSchema{Type: "object", Schema: schema}
	response, err := llm.Complete(ctx, model, request)
	result := CompletionObjectResult[T]{Response: response}
	if err != nil {
		return result, &agent.StructuredOutputError{
			Kind: agent.StructuredOutputErrorKindPrompt, Reason: "completion failed", Cause: err,
		}
	}
	if response == nil || strings.TrimSpace(response.Text) == "" {
		return result, &agent.StructuredOutputError{
			Kind: agent.StructuredOutputErrorKindEmpty, Path: "$", Reason: "is empty",
		}
	}
	if err := json.Unmarshal([]byte(response.Text), &result.Object); err != nil {
		return result, &agent.StructuredOutputError{
			Kind: agent.StructuredOutputErrorKindJSONDecode, Path: "$",
			Reason: "is invalid JSON: " + err.Error(), Cause: err,
		}
	}
	return result, nil
}
