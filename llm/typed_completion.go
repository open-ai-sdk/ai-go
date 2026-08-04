package llm

import (
	"context"

	"github.com/open-ai-sdk/ai-go/tool"
)

// CompletionObjectResult is the decoded value and provider response from one
// direct structured completion.
type CompletionObjectResult[T any] struct {
	Object   T
	Response *CompletionResponse
}

// CompleteObject performs one typed completion from an explicit request.
func CompleteObject[T any](
	ctx context.Context,
	model Model,
	request CompletionRequest,
) (CompletionObjectResult[T], error) {
	schema, err := tool.StrictSchema[T]()
	if err != nil {
		return CompletionObjectResult[T]{}, &StructuredOutputError{
			Kind:   StructuredOutputErrorKindPrompt,
			Reason: "invalid output request",
			Cause:  err,
		}
	}
	request.Output = &OutputSchema{Type: "object", Schema: schema}
	response, err := Complete(ctx, model, request)
	result := CompletionObjectResult[T]{Response: response}
	if err != nil {
		return result, &StructuredOutputError{
			Kind:   StructuredOutputErrorKindPrompt,
			Reason: "completion failed",
			Cause:  err,
		}
	}
	if response == nil {
		return result, &StructuredOutputError{Kind: StructuredOutputErrorKindEmpty, Path: "$", Reason: "is empty"}
	}
	result.Object, err = DecodeStructured[T](response.Text, request.Output)
	return result, err
}

// TypedCompletion is the direct-completion lifecycle for a typed response.
// It performs exactly one model call and does not run an Agent or retain
// extraction configuration.
type TypedCompletion[T any] struct {
	model   Model
	request CompletionRequest
}

// NewTypedCompletion starts a typed direct completion with prompt appended as
// a user message.
func NewTypedCompletion[T any](model Model, prompt string) TypedCompletion[T] {
	return TypedCompletion[T]{model: model, request: NewCompletion(model, prompt).Build()}
}

// PromptTyped is the minimal typed-completion convenience. Prompt is the only
// per-call input.
func PromptTyped[T any](ctx context.Context, model Model, prompt string) (CompletionObjectResult[T], error) {
	return NewTypedCompletion[T](model, prompt).Complete(ctx)
}

func (b TypedCompletion[T]) Instructions(value string) TypedCompletion[T] {
	b.request = CompletionFromRequest(b.model, b.request).Instructions(value).Build()
	return b
}

func (b TypedCompletion[T]) Settings(value CallSettings) TypedCompletion[T] {
	b.request = CompletionFromRequest(b.model, b.request).Settings(value).Build()
	return b
}

func (b TypedCompletion[T]) ProviderOptionsJSON(provider string, value map[string]any) TypedCompletion[T] {
	b.request = CompletionFromRequest(b.model, b.request).ProviderOptionsJSON(provider, value).Build()
	return b
}

func (b TypedCompletion[T]) With(option ProviderOption) TypedCompletion[T] {
	b.request = CompletionFromRequest(b.model, b.request).With(option).Build()
	return b
}

// Complete performs one model call and returns T plus the normalized response.
func (b TypedCompletion[T]) Complete(ctx context.Context) (CompletionObjectResult[T], error) {
	return CompleteObject[T](ctx, b.model, b.request)
}
