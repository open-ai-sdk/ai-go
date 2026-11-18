package kie

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/provider/openaicompat"
)

func (m *LanguageModel) normalizeLanguageResponse(raw map[string]any) (*llm.CompletionResponse, error) {
	response := &llm.CompletionResponse{
		Message: aikit.Message{Role: aikit.RoleAssistant}, RawResponse: raw,
		FinishReason: aikit.FinishReasonStop,
	}
	payload, err := languageResponsePayload(response, raw)
	if err != nil {
		return response, err
	}
	response.ProviderMetadata = map[string]any{"kie": map[string]any{"id": payload["id"], "model": payload["model"]}}
	if code, ok := payload["code"].(float64); ok && code >= 400 {
		message := languageValue[string](payload["msg"])
		response.FinishReason = aikit.FinishReasonError
		return response, &aikit.APIError{Provider: "kie", Code: fmt.Sprint(code), Message: message}
	}
	usage := languageValue[map[string]any](payload["usage"])
	response.Usage = languageUsage(usage, m.chat)
	if failure := payload["error"]; failure != nil {
		response.FinishReason = aikit.FinishReasonError
		response.RawFinishReason = languageValue[string](payload["status"])
		return response, languageResponseError("response failed", failure)
	}
	if m.chat {
		return normalizeLanguageChat(response, payload)
	}
	response.RawFinishReason = languageValue[string](payload["status"])
	switch response.RawFinishReason {
	case "failed", "cancelled":
		response.FinishReason = aikit.FinishReasonError
		return response, languageResponseError("response "+response.RawFinishReason, payload["error"])
	case "incomplete":
		response.FinishReason = aikit.FinishReasonLength
		return response, languageResponseError("response incomplete", payload["incomplete_details"])
	}
	return normalizeLanguageOutput(response, payload)
}

func languageResponsePayload(response *llm.CompletionResponse, raw map[string]any) (map[string]any, error) {
	events, ok := raw["events"].([]any)
	if !ok {
		return raw, nil
	}
	var payload map[string]any
	for _, entry := range events {
		event := languageValue[map[string]any](entry)
		switch event["type"] {
		case "response.completed", "response.failed", "response.incomplete":
			payload = languageValue[map[string]any](event["response"])
			if payload == nil {
				return nil, fmt.Errorf("kie: %v event missing response field", event["type"])
			}
		case "error":
			response.FinishReason = aikit.FinishReasonError
			response.RawFinishReason = "error"
			return nil, languageResponseError("response stream failed", event)
		}
	}
	if payload == nil {
		return nil, fmt.Errorf("kie: response stream did not include a response.completed event")
	}
	return payload, nil
}

func normalizeLanguageOutput(
	response *llm.CompletionResponse,
	payload map[string]any,
) (*llm.CompletionResponse, error) {
	var text, reasoning strings.Builder
	output := languageValue[[]any](payload["output"])
	for _, entry := range output {
		item := languageValue[map[string]any](entry)
		switch item["type"] {
		case "message":
			if role := languageValue[string](item["role"]); role != "" && role != "assistant" {
				return response, fmt.Errorf("kie: response returned message role %q", role)
			}
			if response.MessageID == "" {
				response.MessageID = languageValue[string](item["id"])
				response.Message.ID = response.MessageID
			}
			parts := languageValue[[]any](item["content"])
			for _, entry := range parts {
				part := languageValue[map[string]any](entry)
				switch part["type"] {
				case "refusal":
					response.FinishReason = aikit.FinishReasonContentFilter
					return response, languageResponseError("response refused", part["refusal"])
				case "output_text":
					value, ok := part["text"].(string)
					if ok {
						text.WriteString(value)
						response.Message.Content = append(
							response.Message.Content,
							aikit.ContentPart{Type: aikit.ContentPartTypeText, Text: value},
						)
					}
				}
			}
		case "reasoning":
			summary := languageValue[[]any](item["summary"])
			for _, entry := range summary {
				part := languageValue[map[string]any](entry)
				value := languageValue[string](part["text"])
				if value != "" {
					reasoning.WriteString(value)
					response.Message.Content = append(
						response.Message.Content,
						aikit.ContentPart{Type: aikit.ContentPartTypeReasoning, ReasoningText: value},
					)
				}
			}
		case "function_call":
			response.Message.Content = append(response.Message.Content, languageToolCall(item, "call_id"))
			response.FinishReason = aikit.FinishReasonToolCalls
		}
	}
	response.Text, response.Reasoning = text.String(), reasoning.String()
	if len(response.Message.Content) == 0 {
		return response, fmt.Errorf("kie: response did not include any assistant content")
	}
	return response, nil
}

func normalizeLanguageChat(response *llm.CompletionResponse, payload map[string]any) (*llm.CompletionResponse, error) {
	choices := languageValue[[]any](payload["choices"])
	if len(choices) == 0 {
		return response, fmt.Errorf("kie: chat response did not include choices")
	}
	choice := languageValue[map[string]any](choices[0])
	message := languageValue[map[string]any](choice["message"])
	if role := languageValue[string](message["role"]); role != "" && role != "assistant" {
		return response, fmt.Errorf("kie: chat response returned message role %q", role)
	}
	response.RawFinishReason = languageValue[string](choice["finish_reason"])
	response.FinishReason = openaicompat.MapFinishReason(response.RawFinishReason)
	if refusal := message["refusal"]; refusal != nil && refusal != "" {
		return response, languageResponseError("chat response refused", refusal)
	}
	if text, ok := message["content"].(string); ok {
		response.Text = text
		response.Message.Content = append(
			response.Message.Content,
			aikit.ContentPart{Type: aikit.ContentPartTypeText, Text: text},
		)
	}
	response.Reasoning = languageValue[string](message["reasoning_content"])
	if response.Reasoning == "" {
		response.Reasoning = languageValue[string](message["reasoning"])
	}
	if response.Reasoning != "" {
		response.Message.Content = append(
			response.Message.Content,
			aikit.ContentPart{Type: aikit.ContentPartTypeReasoning, ReasoningText: response.Reasoning},
		)
	}
	calls := languageValue[[]any](message["tool_calls"])
	for _, entry := range calls {
		call := languageValue[map[string]any](entry)
		function := languageValue[map[string]any](call["function"])
		part := languageToolCall(function, "id")
		part.ToolCallID = languageValue[string](call["id"])
		response.Message.Content = append(response.Message.Content, part)
	}
	if len(response.Message.Content) == 0 {
		return response, fmt.Errorf("kie: chat response did not include any assistant content")
	}
	return response, nil
}

func languageToolCall(call map[string]any, idKey string) aikit.ContentPart {
	id := languageValue[string](call[idKey])
	name := languageValue[string](call["name"])
	arguments := languageValue[string](call["arguments"])
	return aikit.ContentPart{
		Type: aikit.ContentPartTypeToolCall, ToolCallID: id,
		ToolCallName: name, ToolCallArgs: json.RawMessage(arguments),
	}
}

func languageResponseError(message string, detail any) error {
	encoded, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("kie: %s: %w", message, err)
	}
	return fmt.Errorf("kie: %s: %s", message, encoded)
}

func languageUsage(raw map[string]any, chat bool) aikit.Usage {
	usage := aikit.Usage{Raw: raw}
	if raw == nil {
		return usage
	}
	inputKey, outputKey := "input_tokens", "output_tokens"
	if chat {
		inputKey, outputKey = "prompt_tokens", "completion_tokens"
	}
	usage.InputTokens = languageTokenCount(raw, inputKey)
	usage.OutputTokens = languageTokenCount(raw, outputKey)
	usage.TotalTokens = languageTokenCount(raw, "total_tokens")
	if usage.OutputTokens == 0 && usage.TotalTokens > usage.InputTokens {
		usage.OutputTokens = usage.TotalTokens - usage.InputTokens
	}
	input := languageValue[map[string]any](raw[inputKey+"_details"])
	output := languageValue[map[string]any](raw[outputKey+"_details"])
	usage.InputTokenDetails.CacheReadTokens = languageTokenCount(input, "cached_tokens")
	usage.InputTokenDetails.CacheWriteTokens = languageTokenCount(input, "cache_write_tokens")
	usage.InputTokenDetails.NoCacheTokens = max(
		0,
		usage.InputTokens-usage.InputTokenDetails.CacheReadTokens-usage.InputTokenDetails.CacheWriteTokens,
	)
	usage.OutputTokenDetails.ReasoningTokens = languageTokenCount(output, "reasoning_tokens")
	usage.OutputTokenDetails.TextTokens = max(0, usage.OutputTokens-usage.OutputTokenDetails.ReasoningTokens)
	return usage
}

func languageTokenCount(value map[string]any, key string) int {
	count := languageValue[float64](value[key])
	return int(count)
}

// languageValue preserves zero-value handling for absent optional JSON fields.
func languageValue[T any](value any) T {
	if typed, ok := value.(T); ok {
		return typed
	}
	var zero T
	return zero
}
