package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/transport"
)

// ChatCompletionResponse is the untranslated successful response returned by
// an OpenAI-compatible Chat Completions endpoint. Raw contains the exact JSON
// payload read from the provider and is never populated for error responses.
type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object,omitempty"`
	Created int64                  `json:"created,omitempty"`
	Model   string                 `json:"model,omitempty"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   *ChatCompletionUsage   `json:"usage,omitempty"`
	Raw     json.RawMessage        `json:"-"`
}

type ChatCompletionChoice struct {
	Index        int                   `json:"index"`
	Message      ChatCompletionMessage `json:"message"`
	FinishReason string                `json:"finish_reason"`
}

type ChatCompletionMessage struct {
	Role             string                   `json:"role"`
	Content          string                   `json:"content"`
	ReasoningContent string                   `json:"reasoning_content,omitempty"`
	Reasoning        string                   `json:"reasoning,omitempty"`
	ToolCalls        []ChatCompletionToolCall `json:"tool_calls,omitempty"`
}

type ChatCompletionToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type ChatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	PromptDetails    *struct {
		CachedTokens     int `json:"cached_tokens"`
		CacheWriteTokens int `json:"cache_write_tokens"`
	} `json:"prompt_tokens_details,omitempty"`
	CompletionDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details,omitempty"`
}

var _ llm.CompletionModel = (*Model)(nil)

// Complete performs one non-streaming Chat Completions request and retains the
// provider response alongside ai-go's normalized assistant content.
func (m *Model) Complete(ctx context.Context, req llm.Request) (*llm.CompletionResponse, error) {
	timeout := m.cfg.Timeout
	if timeout == 0 && m.cfg.HTTPClient == nil {
		timeout = 120 * time.Second
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	name := providerName(m.cfg.Provider)
	body, warnings, err := m.prepareRequest(req, false)
	if err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorRequest, "complete", name, err)
	}
	if m.clientErr != nil {
		return nil, llm.WrapCompletionError(
			llm.CompletionErrorRequest,
			"complete",
			name,
			fmt.Errorf("%s: configure transport: %w", name, m.clientErr),
		)
	}
	httpReq, err := m.client.NewRequest(ctx, http.MethodPost, "chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorRequest, "complete", name, err)
	}
	httpResp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorTransport, "complete", name, err)
	}
	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		return nil, llm.WrapCompletionError(
			llm.CompletionErrorProvider,
			"complete",
			name,
			transport.APIErrorFromResponse(ctx, name, httpResp),
		)
	}
	defer httpResp.Body.Close()
	raw, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorTransport, "complete", name, err)
	}
	var native ChatCompletionResponse
	if err := json.Unmarshal(raw, &native); err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorJSON, "complete", name, err)
	}
	native.Raw = append(json.RawMessage(nil), raw...)
	response, err := normalizeChatCompletion(&native, warnings)
	if err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorResponse, "complete", name, err)
	}
	return response, nil
}

func normalizeChatCompletion(
	native *ChatCompletionResponse,
	warnings []aikit.Warning,
) (*llm.CompletionResponse, error) {
	if native == nil || len(native.Choices) == 0 {
		return nil, fmt.Errorf("chat completion returned no choices")
	}
	choice := native.Choices[0]
	if choice.Message.Role != "" && choice.Message.Role != "assistant" {
		return nil, fmt.Errorf("chat completion returned role %q", choice.Message.Role)
	}
	message := aikit.Message{Role: aikit.RoleAssistant}
	text := choice.Message.Content
	if text != "" {
		message.Content = append(message.Content, aikit.ContentPart{Type: aikit.ContentPartTypeText, Text: text})
	}
	reasoning := choice.Message.ReasoningContent
	if reasoning == "" {
		reasoning = choice.Message.Reasoning
	}
	if reasoning != "" {
		message.Content = append(message.Content, aikit.ContentPart{
			Type: aikit.ContentPartTypeReasoning, ReasoningText: reasoning,
		})
	}
	for _, call := range choice.Message.ToolCalls {
		message.Content = append(message.Content, aikit.ContentPart{
			Type: aikit.ContentPartTypeToolCall, ToolCallID: call.ID,
			ToolCallName: call.Function.Name,
			ToolCallArgs: append(json.RawMessage(nil), call.Function.Arguments...),
		})
	}
	finishReason := MapFinishReason(choice.FinishReason)
	if len(message.Content) == 0 && finishReason != aikit.FinishReasonLength {
		return nil, fmt.Errorf("chat completion returned no assistant content")
	}
	response := &llm.CompletionResponse{
		Message: message, Text: text, Reasoning: reasoning,
		FinishReason: finishReason, RawFinishReason: choice.FinishReason,
		Warnings: append([]aikit.Warning(nil), warnings...), RawResponse: native,
	}
	if native.Usage != nil {
		response.Usage = normalizeChatUsage(native.Usage)
	}
	return response, nil
}

func normalizeChatUsage(raw *ChatCompletionUsage) aikit.Usage {
	if raw == nil {
		return aikit.Usage{}
	}
	usage := aikit.Usage{
		InputTokens: raw.PromptTokens, OutputTokens: raw.CompletionTokens,
		TotalTokens: raw.TotalTokens,
		Raw: map[string]any{
			"prompt_tokens": raw.PromptTokens, "completion_tokens": raw.CompletionTokens,
			"total_tokens": raw.TotalTokens,
		},
	}
	if usage.OutputTokens == 0 && usage.TotalTokens > usage.InputTokens {
		usage.OutputTokens = usage.TotalTokens - usage.InputTokens
	}
	if raw.PromptDetails != nil {
		usage.InputTokenDetails.CacheReadTokens = raw.PromptDetails.CachedTokens
		usage.InputTokenDetails.CacheWriteTokens = raw.PromptDetails.CacheWriteTokens
	}
	usage.InputTokenDetails.NoCacheTokens = usage.InputTokens -
		usage.InputTokenDetails.CacheReadTokens - usage.InputTokenDetails.CacheWriteTokens
	if usage.InputTokenDetails.NoCacheTokens < 0 {
		usage.InputTokenDetails.NoCacheTokens = 0
	}
	if raw.CompletionDetails != nil {
		usage.OutputTokenDetails.ReasoningTokens = raw.CompletionDetails.ReasoningTokens
	}
	usage.OutputTokenDetails.TextTokens = usage.OutputTokens - usage.OutputTokenDetails.ReasoningTokens
	if usage.OutputTokenDetails.TextTokens < 0 {
		usage.OutputTokenDetails.TextTokens = 0
	}
	return usage
}
