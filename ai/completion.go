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

// Streaming adapts a model to the streaming entrypoints, for code that must
// accept anything streamable rather than make one call.
func Streaming(model LanguageModel) ModelStream {
	return llm.Streaming(model)
}

// StreamPrompt streams one direct user prompt and carries the aggregate it
// produced. It is the streaming twin of Prompt.
func StreamPrompt(ctx context.Context, model LanguageModel, prompt string) (*StreamingResponse, error) {
	return llm.StreamPrompt(ctx, model, prompt)
}

// StreamChat streams one direct user prompt after history. It is the streaming
// twin of Chat.
func StreamChat(
	ctx context.Context,
	model LanguageModel,
	prompt string,
	history ...Message,
) (*StreamingResponse, error) {
	return llm.StreamChat(ctx, model, prompt, history...)
}
