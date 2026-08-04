package llm

import (
	"context"
	"errors"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// errNilCompletionModel is the one wording every entrypoint reports when no
// model is bound, so Send, StreamSend, and StreamCompletion cannot drift apart.
var errNilCompletionModel = errors.New("llm: completion model is required")

// CompletionRequest is the normalized input for one direct model call.
// It is an alias of Request so provider implementations have one canonical
// request contract.
type CompletionRequest = Request

// CompletionResponse is the aggregated result of one direct model call.
// Message is suitable for appending to a later Request.Messages value. Tool
// calls are returned as message parts but are never executed by this package.
type CompletionResponse struct {
	Message          aikit.Message
	MessageID        string
	Text             string
	Reasoning        string
	Usage            aikit.Usage
	RawResponse      any
	FinishReason     aikit.FinishReason
	RawFinishReason  string
	ProviderMetadata map[string]any
	Warnings         []aikit.Warning
	Sources          []aikit.Source
	Files            []GeneratedFile
}

// RawResponseAs returns the provider-native successful response when it has
// the requested type. Raw responses are diagnostic data and may contain
// sensitive provider fields; ai-go never logs them automatically.
func RawResponseAs[T any](response *CompletionResponse) (T, bool) {
	var zero T
	if response == nil || response.RawResponse == nil {
		return zero, false
	}
	value, ok := response.RawResponse.(T)
	return value, ok
}

// GeneratedFile is a file emitted by a provider during a completion.
type GeneratedFile struct {
	Data      []byte
	MediaType string
}

// CompletionRequestBuilder binds a normalized request to a model while keeping
// ai-go's stream-first model contract. All methods return a new top-level
// builder value.
type CompletionRequestBuilder struct {
	model   Model
	request Request
}

// NewCompletion starts a direct completion request with prompt appended as a
// user message.
func NewCompletion(model Model, prompt string) CompletionRequestBuilder {
	return CompletionRequestBuilder{model: model, request: NewRequest(prompt).Build()}
}

// CompletionFromRequest starts a direct completion request from explicit
// defaults.
func CompletionFromRequest(model Model, defaults CompletionRequest) CompletionRequestBuilder {
	return CompletionRequestBuilder{model: model, request: cloneRequest(defaults)}
}

func (b CompletionRequestBuilder) Model(model Model) CompletionRequestBuilder {
	b.model = model
	return b
}

func (b CompletionRequestBuilder) Instructions(value string) CompletionRequestBuilder {
	b.request.Instructions = value
	return b
}

func (b CompletionRequestBuilder) Messages(values ...aikit.Message) CompletionRequestBuilder {
	b.request.Messages = append([]aikit.Message(nil), values...)
	return b
}

func (b CompletionRequestBuilder) Prompt(value string) CompletionRequestBuilder {
	b.request.Messages = append(append([]aikit.Message(nil), b.request.Messages...), aikit.Message{
		Role: aikit.RoleUser, Content: []aikit.ContentPart{{Type: aikit.ContentPartTypeText, Text: value}},
	})
	return b
}

func (b CompletionRequestBuilder) Tools(values ...aikit.ToolDefinition) CompletionRequestBuilder {
	b.request.Tools = append([]aikit.ToolDefinition(nil), values...)
	return b
}

func (b CompletionRequestBuilder) ToolChoice(value aikit.ToolChoice) CompletionRequestBuilder {
	b.request.ToolChoice = &value
	return b
}

func (b CompletionRequestBuilder) Output(value OutputSchema) CompletionRequestBuilder {
	b.request.Output = &value
	return b
}

func (b CompletionRequestBuilder) Settings(value CallSettings) CompletionRequestBuilder {
	b.request.Settings = cloneCallSettings(value)
	return b
}

func (b CompletionRequestBuilder) Temperature(value float32) CompletionRequestBuilder {
	b.request.Settings.Temperature = &value
	return b
}

func (b CompletionRequestBuilder) MaxTokens(value int) CompletionRequestBuilder {
	b.request.Settings.MaxTokens = value
	return b
}

func (b CompletionRequestBuilder) TopP(value float32) CompletionRequestBuilder {
	b.request.Settings.TopP = &value
	return b
}

func (b CompletionRequestBuilder) TopK(value int) CompletionRequestBuilder {
	b.request.Settings.TopK = &value
	return b
}

func (b CompletionRequestBuilder) Seed(value int) CompletionRequestBuilder {
	b.request.Settings.Seed = &value
	return b
}

func (b CompletionRequestBuilder) StopSequences(values ...string) CompletionRequestBuilder {
	b.request.Settings.StopSequences = append([]string(nil), values...)
	return b
}

func (b CompletionRequestBuilder) ProviderOptionsJSON(
	provider string,
	options map[string]any,
) CompletionRequestBuilder {
	b.request.ProviderOptions = withProviderOption(b.request.ProviderOptions, provider, cloneMap(options))
	return b
}

func (b CompletionRequestBuilder) With(option ProviderOption) CompletionRequestBuilder {
	if !IsNilProviderOption(option) {
		b.request.ProviderOptions = withProviderOption(b.request.ProviderOptions, option.ProviderName(), option)
	}
	return b
}

func (b CompletionRequestBuilder) ToolsContext(value aikit.ToolsContext) CompletionRequestBuilder {
	b.request.ToolsContext = cloneMap(value)
	return b
}

func (b CompletionRequestBuilder) RuntimeContext(value aikit.RuntimeContext) CompletionRequestBuilder {
	b.request.RuntimeContext = cloneMap(value)
	return b
}

// Build returns an independent top-level request value.
func (b CompletionRequestBuilder) Build() CompletionRequest { return cloneRequest(b.request) }

// StreamSend starts exactly one provider model call and returns both its event
// sequence and the aggregate that sequence builds. It does not invoke tools or
// apply agent stop conditions.
//
// The provider call is made on the first pull from Events, so a response that
// is never ranged opens no connection; Close releases it. Request validation
// still happens here and is reported immediately.
func (b CompletionRequestBuilder) StreamSend(ctx context.Context) (*StreamingResponse, error) {
	if b.model == nil {
		return nil, &CompletionError{
			Kind: CompletionErrorKindRequest, Operation: "stream",
			Cause: errNilCompletionModel,
		}
	}
	return newStreamingResponse(ctx, b.model, b.Build()), nil
}

// Send runs exactly one provider model call and aggregates its normalized
// events. If the provider emits an error event, the partial response and that
// error are returned together.
func (b CompletionRequestBuilder) Send(ctx context.Context) (*CompletionResponse, error) {
	if b.model == nil {
		return nil, NewCompletionError(CompletionErrorKindRequest, "send", "", errNilCompletionModel)
	}
	if model, ok := b.model.(CompletionModel); ok {
		response, err := model.Complete(ctx, b.Build())
		if err != nil {
			return response, wrapCompletionError(err, CompletionErrorKindProvider, "complete")
		}
		if response == nil {
			return nil, invalidCompletionResponse("complete", "model returned a nil response")
		}
		return response, nil
	}
	stream, err := b.StreamSend(ctx)
	if err != nil {
		return nil, err
	}
	for range stream.Events() {
	}
	response, err := stream.Response()
	if err != nil {
		return response, wrapCompletionError(err, CompletionErrorKindProvider, "collect")
	}
	return response, nil
}

// Complete is the explicit function form of CompletionRequestBuilder.Send.
func Complete(ctx context.Context, model Model, request CompletionRequest) (*CompletionResponse, error) {
	return CompletionFromRequest(model, request).Send(ctx)
}

// Prompt sends one direct user prompt and returns its aggregated text. For
// tools, reasoning, sources, or continuation messages, use NewCompletion.
func Prompt(ctx context.Context, model Model, prompt string) (string, error) {
	response, err := NewCompletion(model, prompt).Send(ctx)
	if response == nil {
		return "", err
	}
	return response.Text, err
}

// Chat sends one direct user prompt after history and returns its aggregated
// text. History is copied into the request; it is never mutated.
func Chat(ctx context.Context, model Model, prompt string, history ...aikit.Message) (string, error) {
	response, err := NewCompletion(model, "").Messages(history...).Prompt(prompt).Send(ctx)
	if response == nil {
		return "", err
	}
	return response.Text, err
}
