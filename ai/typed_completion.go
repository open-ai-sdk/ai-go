package ai

import (
	"context"

	"github.com/open-ai-sdk/ai-go/llm"
)

// TypedCompletion is the direct-completion lifecycle for a typed response.
// It belongs beside Completion because it performs exactly one model call and
// does not run an Agent or reuse extraction configuration.
type TypedCompletion[T any] struct {
	model   llm.Model
	request llm.CompletionRequest
}

// NewTypedCompletion starts a typed direct completion with prompt appended as
// a user message.
func NewTypedCompletion[T any](model llm.Model, prompt string) TypedCompletion[T] {
	return TypedCompletion[T]{model: model, request: llm.NewCompletion(model, prompt).Build()}
}

// PromptTyped is the minimal typed-completion convenience. It mirrors a typed
// prompt on a configured client: prompt is the only per-call input.
func PromptTyped[T any](ctx context.Context, model llm.Model, prompt string) (CompletionObjectResult[T], error) {
	return NewTypedCompletion[T](model, prompt).Complete(ctx)
}

func (b TypedCompletion[T]) Instructions(value string) TypedCompletion[T] {
	b.request = llm.CompletionFromRequest(b.model, b.request).Instructions(value).Build()
	return b
}
func (b TypedCompletion[T]) Settings(value llm.CallSettings) TypedCompletion[T] {
	b.request = llm.CompletionFromRequest(b.model, b.request).Settings(value).Build()
	return b
}
func (b TypedCompletion[T]) ProviderOptionsJSON(provider string, value map[string]any) TypedCompletion[T] {
	b.request = llm.CompletionFromRequest(b.model, b.request).ProviderOptionsJSON(provider, value).Build()
	return b
}
func (b TypedCompletion[T]) With(option llm.ProviderOption) TypedCompletion[T] {
	b.request = llm.CompletionFromRequest(b.model, b.request).With(option).Build()
	return b
}

// Complete performs one model call and returns T plus the normalized response.
func (b TypedCompletion[T]) Complete(ctx context.Context) (CompletionObjectResult[T], error) {
	return CompleteObject[T](ctx, b.model, b.request)
}
