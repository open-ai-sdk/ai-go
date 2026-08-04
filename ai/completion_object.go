package ai

import (
	"context"

	"github.com/open-ai-sdk/ai-go/llm"
)

type (
	CompletionObjectResult[T any] = llm.CompletionObjectResult[T]
	TypedCompletion[T any]        = llm.TypedCompletion[T]
)

func CompleteObject[T any](ctx context.Context, model LanguageModel, request CompletionRequest) (CompletionObjectResult[T], error) {
	return llm.CompleteObject[T](ctx, model, request)
}

func NewTypedCompletion[T any](model LanguageModel, prompt string) TypedCompletion[T] {
	return llm.NewTypedCompletion[T](model, prompt)
}

func PromptTyped[T any](ctx context.Context, model LanguageModel, prompt string) (CompletionObjectResult[T], error) {
	return llm.PromptTyped[T](ctx, model, prompt)
}
