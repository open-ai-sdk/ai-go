package kie

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/transport"
)

// LanguageModel implements Kie's caller-selected Responses or Chat endpoint.
// Endpoint selection never depends on the model ID. Both Complete and Stream
// issue a non-streaming request; Stream emits the completed normalized parts.
// Responses endpoints may nevertheless return SSE, which Complete also accepts.
type LanguageModel struct {
	provider *Provider
	modelID  string
	path     string
	chat     bool
}

// LanguageOption configures a language model without changing Provider defaults.
type LanguageOption func(*LanguageModel)

// WithLanguagePath sets the exact endpoint path relative to Config.BaseURL.
// Unlike the media APIs, language paths are not rewritten for proxy mirrors.
func WithLanguagePath(path string) LanguageOption {
	return func(model *LanguageModel) { model.path = path }
}

// Responses constructs a Responses model. The generic default path is
// "responses"; pass WithLanguagePath for a gateway-specific endpoint.
func (p *Provider) Responses(modelID string, opts ...LanguageOption) *LanguageModel {
	model := &LanguageModel{provider: p, modelID: modelID, path: "responses"}
	for _, opt := range opts {
		opt(model)
	}
	return model
}

// Chat constructs a Chat Completions model using an explicit endpoint path,
// for example "model-id/v1/chat/completions". The model ID is metadata only:
// Kie's Chat request body does not carry a model field.
func (p *Provider) Chat(modelID, path string, opts ...LanguageOption) *LanguageModel {
	model := &LanguageModel{provider: p, modelID: modelID, path: path, chat: true}
	for _, opt := range opts {
		opt(model)
	}
	return model
}

func (m *LanguageModel) ModelID() string { return m.modelID }

func (*LanguageModel) NativeSchemaSupport() llm.NativeSchemaSupport { return llm.NativeSchemaFull }

// Complete retains the complete JSON object in RawResponse. An SSE response is
// retained as map[string]any{"events": []any{...}}, including nonterminal events.
// A decoded provider failure returns a partial response with the same raw data.
// HTTP errors preserve status and opt-in raw body through transport.APIErrorFromResponse.
func (m *LanguageModel) Complete(ctx context.Context, req llm.Request) (*llm.CompletionResponse, error) {
	wrap := func(kind llm.CompletionErrorKind, err error) error {
		return llm.WrapCompletionError(kind, "complete", "kie", err)
	}
	if strings.TrimSpace(m.path) == "" {
		return nil, wrap(llm.CompletionErrorRequest, fmt.Errorf("kie: language endpoint path is required"))
	}
	body, warnings, err := m.languageRequestBody(req)
	if err != nil {
		return nil, wrap(llm.CompletionErrorRequest, err)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, wrap(llm.CompletionErrorRequest, err)
	}
	if m.provider.clientErr != nil {
		return nil, wrap(llm.CompletionErrorRequest, m.provider.clientErr)
	}
	ctx, cancel := context.WithTimeout(ctx, m.provider.cfg.Timeout)
	defer cancel()
	httpReq, err := m.provider.client.NewRequest(ctx, http.MethodPost, m.path, bytes.NewReader(encoded))
	if err != nil {
		return nil, wrap(llm.CompletionErrorRequest, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpResp, err := m.provider.client.Do(httpReq)
	if err != nil {
		return nil, wrap(llm.CompletionErrorTransport, err)
	}
	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		return nil, wrap(llm.CompletionErrorProvider, transport.APIErrorFromResponse(ctx, "kie", httpResp))
	}
	defer httpResp.Body.Close()
	raw, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, wrap(llm.CompletionErrorTransport, err)
	}
	payload, err := decodeLanguagePayload(raw)
	if err != nil {
		response := &llm.CompletionResponse{RawResponse: payload, Warnings: warnings}
		return response, wrap(llm.CompletionErrorJSON, err)
	}
	response, err := m.normalizeLanguageResponse(payload)
	response.Warnings = warnings
	if err != nil {
		var providerError *aikit.APIError
		if errors.As(err, &providerError) {
			return response, wrap(llm.CompletionErrorProvider, err)
		}
		return response, wrap(llm.CompletionErrorResponse, err)
	}
	return response, nil
}

// Stream adapts a single completed response to Model's normalized event
// contract. It does not request incremental generation from the gateway.
func (m *LanguageModel) Stream(ctx context.Context, req llm.Request) (<-chan aikit.StreamEvent, error) {
	response, err := m.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	events := make(chan aikit.StreamEvent)
	go func() {
		defer close(events)
		emit := func(event aikit.StreamEvent) bool {
			select {
			case <-ctx.Done():
				return false
			case events <- event:
				return true
			}
		}
		toolIndex := 0
		for _, part := range response.Message.Content {
			var event aikit.StreamEvent
			switch part.Type {
			case aikit.ContentPartTypeText:
				event = aikit.StreamEvent{Type: aikit.StreamEventTextDelta, TextDelta: part.Text}
			case aikit.ContentPartTypeReasoning:
				event = aikit.StreamEvent{Type: aikit.StreamEventReasoningDelta, TextDelta: part.ReasoningText}
			case aikit.ContentPartTypeToolCall:
				event = aikit.StreamEvent{
					Type:              aikit.StreamEventToolCallDelta,
					ToolCallIndex:     toolIndex,
					ToolCallID:        part.ToolCallID,
					ToolCallName:      part.ToolCallName,
					ToolCallArgsDelta: string(part.ToolCallArgs),
				}
				toolIndex++
			default:
				continue
			}
			if !emit(event) {
				return
			}
		}
		if !emit(aikit.StreamEvent{Type: aikit.StreamEventUsage, Usage: &response.Usage}) {
			return
		}
		emit(aikit.StreamEvent{
			Type: aikit.StreamEventFinish, MessageID: response.MessageID,
			FinishReason: response.FinishReason, RawFinishReason: response.RawFinishReason,
			ProviderMetadata: response.ProviderMetadata, Warnings: response.Warnings,
		})
	}()
	return events, nil
}

var _ llm.CompletionModel = (*LanguageModel)(nil)

func decodeLanguagePayload(raw []byte) (map[string]any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var payload map[string]any
		err := json.Unmarshal(trimmed, &payload)
		return payload, err
	}
	reader := transport.NewSSEReader(bytes.NewReader(raw))
	events := make([]any, 0)
	payload := map[string]any{"events": events}
	for {
		frame, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return payload, nil
		}
		if err != nil {
			return payload, err
		}
		if frame.Data == "" || frame.Data == "[DONE]" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(frame.Data), &event); err != nil {
			return payload, err
		}
		events = append(events, event)
		payload["events"] = events
	}
}
