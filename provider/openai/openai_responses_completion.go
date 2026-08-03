package openai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/transport"
)

// ResponsesResponse is the provider-native successful OpenAI Responses API
// payload. Raw retains the exact JSON bytes for diagnostics and forward
// compatibility with fields not yet represented by this DTO.
type ResponsesResponse struct {
	ID                string                     `json:"id"`
	Object            string                     `json:"object,omitempty"`
	Model             string                     `json:"model,omitempty"`
	Status            string                     `json:"status"`
	IncompleteDetails *ResponsesIncompleteDetail `json:"incomplete_details,omitempty"`
	Output            []ResponsesOutputItem      `json:"output"`
	Usage             *ResponsesUsage            `json:"usage,omitempty"`
	Raw               json.RawMessage            `json:"-"`
}

type ResponsesIncompleteDetail struct {
	Reason string `json:"reason"`
}

type ResponsesOutputItem struct {
	ID        string                      `json:"id,omitempty"`
	Type      string                      `json:"type"`
	Role      string                      `json:"role,omitempty"`
	Content   []ResponsesOutputContent    `json:"content,omitempty"`
	Summary   []ResponsesReasoningSummary `json:"summary,omitempty"`
	CallID    string                      `json:"call_id,omitempty"`
	Name      string                      `json:"name,omitempty"`
	Arguments string                      `json:"arguments,omitempty"`
	Result    string                      `json:"result,omitempty"`
}

type ResponsesOutputContent struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Refusal string `json:"refusal,omitempty"`
}

type ResponsesReasoningSummary struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
	InputDetails *struct {
		CachedTokens     int `json:"cached_tokens"`
		CacheWriteTokens int `json:"cache_write_tokens"`
	} `json:"input_tokens_details,omitempty"`
	OutputDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details,omitempty"`
}

var _ llm.CompletionModel = (*LanguageModel)(nil)

// Complete performs one non-streaming Responses API call.
func (m *LanguageModel) Complete(ctx context.Context, req llm.Request) (*llm.CompletionResponse, error) {
	if m.clientErr != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorRequest, "complete", "openai", m.clientErr)
	}
	ctx, cancel := context.WithTimeout(ctx, m.client.timeout)
	defer cancel()
	apiReq, warnings, err := encodeRequest(m.modelID, req, false)
	if err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorRequest, "complete", "openai", err)
	}
	httpReq, err := m.buildRequest(ctx, apiReq)
	if err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorRequest, "complete", "openai", err)
	}
	httpResp, err := m.client.responses.Do(httpReq)
	if err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorTransport, "complete", "openai", err)
	}
	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		return nil, llm.WrapCompletionError(
			llm.CompletionErrorProvider,
			"complete",
			"openai",
			transport.APIErrorFromResponse(ctx, "openai", httpResp),
		)
	}
	defer httpResp.Body.Close()
	raw, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorTransport, "complete", "openai", err)
	}
	var native ResponsesResponse
	if err := json.Unmarshal(raw, &native); err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorJSON, "complete", "openai", err)
	}
	native.Raw = append(json.RawMessage(nil), raw...)
	response, err := normalizeResponsesResponse(&native, warnings)
	if err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorResponse, "complete", "openai", err)
	}
	return response, nil
}

func normalizeResponsesResponse(
	native *ResponsesResponse,
	warnings []aikit.Warning,
) (*llm.CompletionResponse, error) {
	if native == nil {
		return nil, fmt.Errorf("responses API returned no response")
	}
	state := responsesNormalization{message: aikit.Message{Role: aikit.RoleAssistant}}
	for _, item := range native.Output {
		if err := state.appendItem(item); err != nil {
			return nil, err
		}
	}
	if len(state.message.Content) == 0 {
		return nil, fmt.Errorf("responses API returned no assistant content")
	}
	state.message.ID = state.messageID
	rawFinishReason := native.Status
	if native.Status == "incomplete" && native.IncompleteDetails != nil && native.IncompleteDetails.Reason != "" {
		rawFinishReason = native.IncompleteDetails.Reason
	}
	response := &llm.CompletionResponse{
		Message: state.message, MessageID: state.messageID, Text: state.text, Reasoning: state.reasoning,
		FinishReason:    mapResponsesFinishReason(rawFinishReason, hasToolCall(state.message.Content)),
		RawFinishReason: rawFinishReason, ProviderMetadata: map[string]any{
			"openai": map[string]any{"responseId": native.ID},
		},
		Warnings: append([]aikit.Warning(nil), warnings...), Files: state.files, RawResponse: native,
	}
	if native.Usage != nil {
		response.Usage = normalizeResponsesUsage(native.Usage)
	}
	return response, nil
}

type responsesNormalization struct {
	message         aikit.Message
	text, reasoning string
	messageID       string
	files           []llm.GeneratedFile
}

func (state *responsesNormalization) appendItem(item ResponsesOutputItem) error {
	switch item.Type {
	case "message":
		if item.Role != "" && item.Role != "assistant" {
			return fmt.Errorf("responses API returned message role %q", item.Role)
		}
		if state.messageID == "" {
			state.messageID = item.ID
		}
		for _, content := range item.Content {
			if content.Type == "output_text" && content.Text != "" {
				state.text += content.Text
				state.message.Content = append(state.message.Content, aikit.ContentPart{
					Type: aikit.ContentPartTypeText, Text: content.Text,
				})
			}
		}
	case "reasoning":
		for _, summary := range item.Summary {
			if summary.Text != "" {
				state.reasoning += summary.Text
				state.message.Content = append(state.message.Content, aikit.ContentPart{
					Type: aikit.ContentPartTypeReasoning, ReasoningText: summary.Text,
				})
			}
		}
	case "function_call":
		state.message.Content = append(state.message.Content, aikit.ContentPart{
			Type: aikit.ContentPartTypeToolCall, ToolCallID: item.CallID,
			ToolCallName: item.Name,
			ToolCallArgs: append(json.RawMessage(nil), item.Arguments...),
		})
	case "image_generation_call":
		if item.Result == "" {
			return nil
		}
		data, err := base64.StdEncoding.DecodeString(item.Result)
		if err != nil {
			return fmt.Errorf("decode image generation result: %w", err)
		}
		state.files = append(state.files, llm.GeneratedFile{
			Data: append([]byte(nil), data...), MediaType: "image/png",
		})
		state.message.Content = append(state.message.Content, aikit.ContentPart{
			Type: aikit.ContentPartTypeFile, Data: append([]byte(nil), data...), MediaType: "image/png",
		})
	}
	return nil
}

func hasToolCall(parts []aikit.ContentPart) bool {
	for _, part := range parts {
		if part.Type == aikit.ContentPartTypeToolCall {
			return true
		}
	}
	return false
}

func normalizeResponsesUsage(raw *ResponsesUsage) aikit.Usage {
	usage := aikit.Usage{
		InputTokens: raw.InputTokens, OutputTokens: raw.OutputTokens, TotalTokens: raw.TotalTokens,
		Raw: map[string]any{
			"input_tokens": raw.InputTokens, "output_tokens": raw.OutputTokens, "total_tokens": raw.TotalTokens,
		},
	}
	if raw.InputDetails != nil {
		usage.InputTokenDetails.CacheReadTokens = raw.InputDetails.CachedTokens
		usage.InputTokenDetails.CacheWriteTokens = raw.InputDetails.CacheWriteTokens
	}
	usage.InputTokenDetails.NoCacheTokens = usage.InputTokens -
		usage.InputTokenDetails.CacheReadTokens - usage.InputTokenDetails.CacheWriteTokens
	if usage.InputTokenDetails.NoCacheTokens < 0 {
		usage.InputTokenDetails.NoCacheTokens = 0
	}
	if raw.OutputDetails != nil {
		usage.OutputTokenDetails.ReasoningTokens = raw.OutputDetails.ReasoningTokens
	}
	usage.OutputTokenDetails.TextTokens = usage.OutputTokens - usage.OutputTokenDetails.ReasoningTokens
	if usage.OutputTokenDetails.TextTokens < 0 {
		usage.OutputTokenDetails.TextTokens = 0
	}
	return usage
}
