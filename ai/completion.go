package ai

import (
	"context"

	"github.com/open-ai-sdk/ai-go/llm"
)

// NewCompletion starts a direct, single model completion. It does not run an
// Agent or execute tools; tool calls remain in the returned assistant message
// for the caller to handle.
func NewCompletion(model LanguageModel, prompt string) CompletionRequestBuilder {
	return llm.NewCompletion(model, prompt)
}

// CompletionFromRequest binds explicit normalized request defaults to a model.
func CompletionFromRequest(model LanguageModel, request CompletionRequest) CompletionRequestBuilder {
	return llm.CompletionFromRequest(model, request)
}

// Complete runs exactly one provider model call and aggregates its normalized
// response events.
func Complete(ctx context.Context, model LanguageModel, request CompletionRequest) (*CompletionResponse, error) {
	return llm.Complete(ctx, model, request)
}

func RawResponseAs[T any](response *CompletionResponse) (T, bool) {
	return llm.RawResponseAs[T](response)
}

// Prompt sends one direct user prompt and returns its aggregated text.
func Prompt(ctx context.Context, model LanguageModel, prompt string) (string, error) {
	return llm.Prompt(ctx, model, prompt)
}

// Chat sends one direct user prompt after history and returns its aggregated text.
func Chat(ctx context.Context, model LanguageModel, prompt string, history ...Message) (string, error) {
	return llm.Chat(ctx, model, prompt, history...)
}
