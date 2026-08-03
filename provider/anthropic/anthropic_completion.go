package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/transport"
)

// MessageResponse is the provider-native successful Anthropic Messages payload.
// Raw retains the exact JSON received from the provider.
type MessageResponse struct {
	ID         string                   `json:"id"`
	Type       string                   `json:"type,omitempty"`
	Role       string                   `json:"role"`
	Model      string                   `json:"model,omitempty"`
	Content    []MessageResponseContent `json:"content"`
	StopReason string                   `json:"stop_reason"`
	Usage      MessageResponseUsage     `json:"usage"`
	Raw        json.RawMessage          `json:"-"`
}

type MessageResponseContent struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
}

type MessageResponseUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

var _ llm.CompletionModel = (*LanguageModel)(nil)

// Complete performs one non-streaming Anthropic Messages request.
func (m *LanguageModel) Complete(ctx context.Context, req llm.Request) (*llm.CompletionResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, m.config.Timeout)
	defer cancel()
	body, warnings, err := m.encodeRequest(req, false)
	if err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorRequest, "complete", "anthropic", err)
	}
	if m.clientErr != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorRequest, "complete", "anthropic", m.clientErr)
	}
	httpReq, err := m.client.NewRequest(ctx, http.MethodPost, "v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorRequest, "complete", "anthropic", err)
	}
	httpResp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorTransport, "complete", "anthropic", err)
	}
	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		return nil, llm.WrapCompletionError(
			llm.CompletionErrorProvider,
			"complete",
			"anthropic",
			transport.APIErrorFromResponse(ctx, "anthropic", httpResp),
		)
	}
	defer httpResp.Body.Close()
	raw, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorTransport, "complete", "anthropic", err)
	}
	var native MessageResponse
	if err := json.Unmarshal(raw, &native); err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorJSON, "complete", "anthropic", err)
	}
	native.Raw = append(json.RawMessage(nil), raw...)
	response, err := normalizeMessageResponse(&native, warnings)
	if err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorResponse, "complete", "anthropic", err)
	}
	return response, nil
}

func normalizeMessageResponse(
	native *MessageResponse,
	warnings []aikit.Warning,
) (*llm.CompletionResponse, error) {
	if native == nil {
		return nil, fmt.Errorf("messages API returned no response")
	}
	if native.Role != "" && native.Role != "assistant" {
		return nil, fmt.Errorf("messages API returned role %q", native.Role)
	}
	warnings = append([]aikit.Warning(nil), warnings...)
	message := aikit.Message{ID: native.ID, Role: aikit.RoleAssistant}
	var text, reasoning string
	for _, content := range native.Content {
		switch content.Type {
		case "text":
			if content.Text != "" {
				text += content.Text
				message.Content = append(message.Content, aikit.ContentPart{
					Type: aikit.ContentPartTypeText, Text: content.Text,
				})
			}
		case "thinking":
			if content.Thinking != "" {
				reasoning += content.Thinking
				message.Content = append(message.Content, aikit.ContentPart{
					Type: aikit.ContentPartTypeReasoning, ReasoningText: content.Thinking,
					ThoughtSignature: content.Signature,
				})
			}
		case "tool_use":
			message.Content = append(message.Content, aikit.ContentPart{
				Type: aikit.ContentPartTypeToolCall, ToolCallID: content.ID,
				ToolCallName: content.Name,
				ToolCallArgs: append(json.RawMessage(nil), content.Input...),
			})
		default:
			warnings = append(warnings, unsupportedResponseBlockWarning(content.Type))
		}
	}
	if len(message.Content) == 0 {
		return nil, fmt.Errorf("messages API returned no assistant content")
	}
	usage := aikit.Usage{
		// Anthropic reports uncached input separately from cache reads and writes.
		// Preserve each provider counter rather than folding cache activity into
		// InputTokens; callers can then compare token categories consistently
		// across direct completions.
		InputTokens:  native.Usage.InputTokens,
		OutputTokens: native.Usage.OutputTokens,
		InputTokenDetails: aikit.InputTokenDetails{
			NoCacheTokens: native.Usage.InputTokens, CacheReadTokens: native.Usage.CacheReadInputTokens,
			CacheWriteTokens: native.Usage.CacheCreationInputTokens,
		},
		Raw: map[string]any{
			"input_tokens": native.Usage.InputTokens, "output_tokens": native.Usage.OutputTokens,
			"cache_read_input_tokens":     native.Usage.CacheReadInputTokens,
			"cache_creation_input_tokens": native.Usage.CacheCreationInputTokens,
		},
	}
	return &llm.CompletionResponse{
		Message: message, MessageID: native.ID, Text: text, Reasoning: reasoning,
		Usage: usage, FinishReason: mapStopReason(native.StopReason), RawFinishReason: native.StopReason,
		Warnings: append([]aikit.Warning(nil), warnings...), RawResponse: native,
	}, nil
}

func unsupportedResponseBlockWarning(blockType string) aikit.Warning {
	return aikit.Warning{
		Type:    "unsupported-response-part",
		Setting: blockType,
		Message: fmt.Sprintf("anthropic: unsupported response content block type %q, skipping", blockType),
	}
}
