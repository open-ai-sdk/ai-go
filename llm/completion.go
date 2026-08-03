// ai-go: file-length-justification: keeps direct completion request building, stream aggregation, and response normalization together.
package llm

import (
	"context"
	"errors"
	"fmt"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// CompletionRequest is the normalized input for one direct model call.
// It is an alias of Request so provider implementations have one canonical
// request contract.
type CompletionRequest = Request

// CompletionResponse is the aggregated result of one direct model call.
// Message is suitable for appending to a later Request.Messages value. Tool
// calls are returned as message parts but are never executed by this package.
type CompletionResponse struct {
	Message          aikit.Message
	Text             string
	Reasoning        string
	Usage            aikit.Usage
	FinishReason     aikit.FinishReason
	RawFinishReason  string
	ProviderMetadata map[string]any
	Warnings         []aikit.Warning
	Sources          []aikit.Source
	Files            []GeneratedFile
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

// Stream starts exactly one provider model call. It does not invoke tools or
// apply agent stop conditions.
func (b CompletionRequestBuilder) Stream(ctx context.Context) (<-chan aikit.StreamEvent, error) {
	if b.model == nil {
		return nil, errors.New("llm: completion model is required")
	}
	return b.model.Stream(ctx, b.Build())
}

// Send runs exactly one provider model call and aggregates its normalized
// events. If the provider emits an error event, the partial response and that
// error are returned together.
func (b CompletionRequestBuilder) Send(ctx context.Context) (*CompletionResponse, error) {
	stream, err := b.Stream(ctx)
	if err != nil {
		return nil, err
	}
	return collectCompletion(stream)
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

func collectCompletion(stream <-chan aikit.StreamEvent) (*CompletionResponse, error) {
	response := &CompletionResponse{Message: aikit.Message{Role: aikit.RoleAssistant}}
	toolParts := make(map[int]int)
	for event := range stream {
		switch event.Type {
		case aikit.StreamEventTextDelta:
			response.Text += event.TextDelta
			appendText(&response.Message.Content, event.TextDelta, event.ThoughtSignature)
		case aikit.StreamEventReasoningDelta:
			response.Reasoning += event.TextDelta
			appendReasoning(&response.Message.Content, event.TextDelta, event.ThoughtSignature)
		case aikit.StreamEventToolCallDelta:
			appendToolCall(&response.Message.Content, toolParts, event)
		case aikit.StreamEventUsage:
			if event.Usage != nil {
				response.Usage = mergeUsage(response.Usage, *event.Usage)
			}
		case aikit.StreamEventSource:
			if event.Source != nil {
				response.Sources = append(response.Sources, *event.Source)
			}
		case aikit.StreamEventFileDelta:
			if len(event.FileData) != 0 {
				data := append([]byte(nil), event.FileData...)
				response.Files = append(response.Files, GeneratedFile{Data: data, MediaType: event.FileMediaType})
				response.Message.Content = append(response.Message.Content, aikit.ContentPart{
					Type:      aikit.ContentPartTypeFile,
					Data:      append([]byte(nil), data...),
					MediaType: event.FileMediaType,
				})
			}
		case aikit.StreamEventFinish:
			response.FinishReason = event.FinishReason
			response.RawFinishReason = event.RawFinishReason
			response.ProviderMetadata = cloneMap(event.ProviderMetadata)
			response.Warnings = append(response.Warnings, event.Warnings...)
		case aikit.StreamEventError:
			if event.Error == nil {
				return response, fmt.Errorf("llm: completion stream emitted a nil error")
			}
			return response, event.Error
		}
	}
	return response, nil
}

func appendText(parts *[]aikit.ContentPart, value, signature string) {
	if value == "" {
		return
	}
	if n := len(*parts); n > 0 &&
		(*parts)[n-1].Type == aikit.ContentPartTypeText &&
		(*parts)[n-1].ThoughtSignature == signature {
		(*parts)[n-1].Text += value
		return
	}
	*parts = append(
		*parts,
		aikit.ContentPart{Type: aikit.ContentPartTypeText, Text: value, ThoughtSignature: signature},
	)
}

func appendReasoning(parts *[]aikit.ContentPart, value, signature string) {
	if value == "" {
		return
	}
	if n := len(*parts); n > 0 &&
		(*parts)[n-1].Type == aikit.ContentPartTypeReasoning &&
		(*parts)[n-1].ThoughtSignature == signature {
		(*parts)[n-1].ReasoningText += value
		return
	}
	*parts = append(
		*parts,
		aikit.ContentPart{Type: aikit.ContentPartTypeReasoning, ReasoningText: value, ThoughtSignature: signature},
	)
}

func appendToolCall(parts *[]aikit.ContentPart, indexes map[int]int, event aikit.StreamEvent) {
	index, found := indexes[event.ToolCallIndex]
	if !found {
		index = len(*parts)
		indexes[event.ToolCallIndex] = index
		*parts = append(
			*parts,
			aikit.ContentPart{
				Type:             aikit.ContentPartTypeToolCall,
				ToolCallID:       event.ToolCallID,
				ToolCallName:     event.ToolCallName,
				ThoughtSignature: event.ThoughtSignature,
			},
		)
	}
	part := &(*parts)[index]
	if event.ToolCallID != "" {
		part.ToolCallID = event.ToolCallID
	}
	if event.ToolCallName != "" {
		part.ToolCallName = event.ToolCallName
	}
	if event.ThoughtSignature != "" {
		part.ThoughtSignature = event.ThoughtSignature
	}
	if event.ToolCallArgsDelta != "" {
		part.ToolCallArgs = append(part.ToolCallArgs, event.ToolCallArgsDelta...)
	}
}

func mergeUsage(prior, incoming aikit.Usage) aikit.Usage {
	take := func(current, next int) int {
		if next != 0 {
			return next
		}
		return current
	}
	merged := prior
	merged.InputTokens = take(prior.InputTokens, incoming.InputTokens)
	merged.OutputTokens = take(prior.OutputTokens, incoming.OutputTokens)
	merged.TotalTokens = take(prior.TotalTokens, incoming.TotalTokens)
	merged.InputTokenDetails.NoCacheTokens = take(
		prior.InputTokenDetails.NoCacheTokens,
		incoming.InputTokenDetails.NoCacheTokens,
	)
	merged.InputTokenDetails.CacheReadTokens = take(
		prior.InputTokenDetails.CacheReadTokens,
		incoming.InputTokenDetails.CacheReadTokens,
	)
	merged.InputTokenDetails.CacheWriteTokens = take(
		prior.InputTokenDetails.CacheWriteTokens,
		incoming.InputTokenDetails.CacheWriteTokens,
	)
	merged.OutputTokenDetails.TextTokens = take(
		prior.OutputTokenDetails.TextTokens,
		incoming.OutputTokenDetails.TextTokens,
	)
	merged.OutputTokenDetails.ReasoningTokens = take(
		prior.OutputTokenDetails.ReasoningTokens,
		incoming.OutputTokenDetails.ReasoningTokens,
	)
	if incoming.Raw != nil {
		merged.Raw = cloneMap(incoming.Raw)
	} else {
		merged.Raw = cloneMap(prior.Raw)
	}
	return merged
}
