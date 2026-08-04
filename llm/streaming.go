package llm

import (
	"context"
	"errors"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// ModelStream adapts any Model to the aikit streaming interfaces. Model itself
// is not widened, so third-party providers keep compiling.
//
// This is the model layer's streaming twin of the Prompt and Chat free
// functions: it closes the asymmetry where a direct call with history could be
// aggregated but not streamed without hand-threading messages through a
// builder. Agent-level streaming is a separate vocabulary and lives in package
// agent.
type ModelStream struct {
	model Model
}

// Streaming wraps a model in the streaming entrypoints. The returned value is
// usable even when model is nil; the failure is reported by each call.
func Streaming(model Model) ModelStream { return ModelStream{model: model} }

// Model returns the wrapped model.
func (m ModelStream) Model() Model { return m.model }

// StreamPrompt streams one direct user prompt. The response carries the
// aggregate the prompt produced, so a caller needing both streamed events and a
// terminal CompletionResponse no longer has to choose.
func (m ModelStream) StreamPrompt(ctx context.Context, prompt string) (*StreamingResponse, error) {
	return NewCompletion(m.model, prompt).StreamSend(ctx)
}

// StreamChat streams one direct user prompt after history. It builds the same
// request Chat builds for the same inputs.
func (m ModelStream) StreamChat(
	ctx context.Context,
	prompt string,
	history ...aikit.Message,
) (*StreamingResponse, error) {
	return NewCompletion(m.model, "").Messages(history...).Prompt(prompt).StreamSend(ctx)
}

// StreamCompletion returns a builder rather than a stream so the caller can
// shape the request — tools, settings, provider options — before sending it.
//
// Prompt and Chat report a missing model by returning an empty string with the
// error. The streaming twins have no such empty value to return, so each
// returns a genuinely nil *StreamingResponse — never a nil pointer boxed in an
// interface — and this one returns the zero builder.
func (m ModelStream) StreamCompletion(
	_ context.Context,
	prompt string,
	history ...aikit.Message,
) (CompletionRequestBuilder, error) {
	if m.model == nil {
		return CompletionRequestBuilder{}, &CompletionError{
			Kind: CompletionErrorKindRequest, Operation: "stream",
			Cause: errors.New("completion model is required"),
		}
	}
	return NewCompletion(m.model, "").Messages(history...).Prompt(prompt), nil
}

// StreamPrompt streams one direct user prompt, mirroring Prompt.
func StreamPrompt(ctx context.Context, model Model, prompt string) (*StreamingResponse, error) {
	return Streaming(model).StreamPrompt(ctx, prompt)
}

// StreamChat streams one direct user prompt after history, mirroring Chat.
// History is copied into the request; it is never mutated.
func StreamChat(
	ctx context.Context,
	model Model,
	prompt string,
	history ...aikit.Message,
) (*StreamingResponse, error) {
	return Streaming(model).StreamChat(ctx, prompt, history...)
}

// The build enforces the contract: removing a method breaks compilation here
// rather than at some consumer.
var (
	_ aikit.StreamingPrompt[aikit.StreamEvent, *StreamingResponse] = ModelStream{}
	_ aikit.StreamingChat[aikit.StreamEvent, *StreamingResponse]   = ModelStream{}
	_ aikit.StreamingCompletion[CompletionRequestBuilder]          = ModelStream{}
	_ aikit.Stream[aikit.StreamEvent]                              = (*StreamingResponse)(nil)
)
