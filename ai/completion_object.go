package ai

import (
	"context"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/tool"
)

// CompletionObjectResult is the decoded value and provider response from one
// direct structured completion.
type CompletionObjectResult[T any] = llm.CompletionObjectResult[T]

// CompleteObject performs exactly one model call and decodes its text as T.
// It never enters the Agent runtime or executes tools.
func CompleteObject[T any](
	ctx context.Context,
	model LanguageModel,
	request CompletionRequest,
) (CompletionObjectResult[T], error) {
	schema, err := tool.StrictSchema[T]()
	if err != nil {
		return CompletionObjectResult[T]{}, &llm.StructuredOutputError{
			Kind: llm.StructuredOutputErrorKindPrompt, Reason: "invalid output request", Cause: err,
		}
	}
	request.Output = &llm.OutputSchema{Type: "object", Schema: schema}
	response, err := llm.Complete(ctx, model, request)
	result := CompletionObjectResult[T]{Response: response}
	if err != nil {
		return result, &llm.StructuredOutputError{
			Kind: llm.StructuredOutputErrorKindPrompt, Reason: "completion failed", Cause: err,
		}
	}
	if response == nil {
		return result, &llm.StructuredOutputError{Kind: llm.StructuredOutputErrorKindEmpty, Path: "$", Reason: "is empty"}
	}
	result.Object, err = llm.DecodeStructured[T](response.Text, request.Output)
	if err != nil {
		return result, err
	}
	return result, nil
}
