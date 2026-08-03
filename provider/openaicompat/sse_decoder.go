package openaicompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/transport"
)

// StreamChunk mirrors the OpenAI chat completions SSE chunk structure.
// Provider-specific delta fields (e.g. Gemini thought flags) are included optionally.
type StreamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content,omitempty"`
			Reasoning        string `json:"reasoning,omitempty"`
			Thought          *bool  `json:"thought,omitempty"`
			ThoughtSignature string `json:"thought_signature,omitempty"`
			ToolCalls        []struct {
				Index        int    `json:"index"`
				ID           string `json:"id"`
				ExtraContent *struct {
					Google struct {
						ThoughtSignature string `json:"thought_signature"`
					} `json:"google"`
				} `json:"extra_content"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		TotalTokens         int `json:"total_tokens"`
		ThoughtsTokenCount  int `json:"thoughts_token_count,omitempty"`
		PromptTokensDetails *struct {
			CachedTokens     int `json:"cached_tokens"`
			CacheWriteTokens int `json:"cache_write_tokens"`
		} `json:"prompt_tokens_details,omitempty"`
		CompletionTokensDetails *struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details,omitempty"`
	} `json:"usage"`
	// ProviderMetadata holds provider-specific metadata from the response (e.g. Gemini groundingMetadata).
	ProviderMetadata map[string]any `json:"provider_metadata,omitempty"`
}

// SSEDecodeParams holds configuration for DecodeSSEStream.
type SSEDecodeParams struct {
	// ProviderName is used in error messages (e.g. "gemini", "openai").
	ProviderName string
	// MetadataExtractor is an optional hook to populate ProviderMetadata on finish events.
	MetadataExtractor func(chunk StreamChunk) map[string]any
	// SourceExtractor is an optional hook to extract aikit.Source events from a chunk.
	// Called for every chunk; returned sources are emitted before text/tool deltas.
	SourceExtractor func(chunk StreamChunk) []aikit.Source
	// EncodeWarnings are advisories raised while encoding the request. They are
	// merged onto the finish event so callers see them in the aggregated result.
	EncodeWarnings []aikit.Warning
}

// DecodeSSEStream reads SSE lines from body and emits normalized aikit.StreamEvents onto ch.
// It closes ch when done or on error.
func DecodeSSEStream(
	ctx context.Context,
	reader *transport.SSEReader,
	ch chan<- aikit.StreamEvent,
	params SSEDecodeParams,
) error {
	providerName := params.ProviderName
	if providerName == "" {
		providerName = "openaichat"
	}

	lineCount := 0
	var finishEmitted bool
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		data, err := reader.NextData()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("%s: read stream: %w", providerName, err)
		}
		lineCount++
		if data == "[DONE]" {
			if !finishEmitted {
				ch <- aikit.StreamEvent{
					Type:            aikit.StreamEventFinish,
					FinishReason:    aikit.FinishReasonStop,
					RawFinishReason: "stop",
					Warnings:        params.EncodeWarnings,
				}
			}
			return nil
		}

		var chunk StreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("%s: unmarshal chunk: %w", providerName, err)
		}

		emitChunkEvents(chunk, ch, params, &finishEmitted)
	}

	if lineCount == 0 {
		return fmt.Errorf("%s: stream ended with zero lines", providerName)
	}
	return nil
}

// emitUsageEvent emits a StreamEventUsage for a chunk that carries token counts.
// Usage may arrive on a chunk with empty choices, so it is handled separately
// from the choice-driven events below.
func emitUsageEvent(chunk StreamChunk, ch chan<- aikit.StreamEvent) {
	if chunk.Usage == nil {
		return
	}
	u := chunk.Usage
	var cachedTokens, cacheWriteTokens int
	if u.PromptTokensDetails != nil {
		cachedTokens = u.PromptTokensDetails.CachedTokens
		cacheWriteTokens = u.PromptTokensDetails.CacheWriteTokens
	}
	reasoningTokens := u.ThoughtsTokenCount
	if u.CompletionTokensDetails != nil && u.CompletionTokensDetails.ReasoningTokens != 0 {
		reasoningTokens = u.CompletionTokensDetails.ReasoningTokens
	}
	// prompt_tokens already includes cached and cache-write tokens; the
	// non-cached remainder is what is left after removing them.
	noCache := u.PromptTokens - cachedTokens - cacheWriteTokens
	if noCache < 0 {
		noCache = 0
	}
	ch <- aikit.StreamEvent{
		Type: aikit.StreamEventUsage,
		Usage: &aikit.Usage{
			InputTokens: u.PromptTokens,
			InputTokenDetails: aikit.InputTokenDetails{
				NoCacheTokens:    noCache,
				CacheReadTokens:  cachedTokens,
				CacheWriteTokens: cacheWriteTokens,
			},
			OutputTokens: u.CompletionTokens,
			OutputTokenDetails: aikit.OutputTokenDetails{
				TextTokens:      u.CompletionTokens - reasoningTokens,
				ReasoningTokens: reasoningTokens,
			},
			TotalTokens: u.TotalTokens,
			Raw: map[string]any{
				"prompt_tokens":     u.PromptTokens,
				"completion_tokens": u.CompletionTokens,
				"total_tokens":      u.TotalTokens,
				"cached_tokens":     cachedTokens,
				"reasoning_tokens":  reasoningTokens,
			},
		},
	}
}

func emitChunkEvents(
	chunk StreamChunk,
	ch chan<- aikit.StreamEvent,
	params SSEDecodeParams,
	finishEmitted *bool,
) {
	metaExtractor := params.MetadataExtractor
	sourceExtractor := params.SourceExtractor

	// Emit usage when present (may arrive on a chunk with empty choices).
	emitUsageEvent(chunk, ch)

	// Emit sources extracted from this chunk (e.g. Gemini grounding chunks).
	if sourceExtractor != nil {
		for _, src := range sourceExtractor(chunk) {
			ch <- aikit.StreamEvent{Type: aikit.StreamEventSource, Source: &src}
		}
	}

	if len(chunk.Choices) == 0 {
		return
	}
	choice := chunk.Choices[0]

	// Emit finish reason if present.
	if choice.FinishReason != "" && choice.FinishReason != "null" {
		var meta map[string]any
		if metaExtractor != nil {
			meta = metaExtractor(chunk)
		}
		ch <- aikit.StreamEvent{
			Type:             aikit.StreamEventFinish,
			FinishReason:     MapFinishReason(choice.FinishReason),
			RawFinishReason:  choice.FinishReason,
			ProviderMetadata: meta,
			Warnings:         params.EncodeWarnings,
		}
		*finishEmitted = true
	}

	// Reasoning delta from reasoning_content / reasoning fields (OpenAI, DeepSeek, xAI, etc.).
	reasoningText := choice.Delta.ReasoningContent
	if reasoningText == "" {
		reasoningText = choice.Delta.Reasoning
	}
	if reasoningText != "" {
		ch <- aikit.StreamEvent{
			Type:      aikit.StreamEventReasoningDelta,
			TextDelta: reasoningText,
		}
	}

	// Text or reasoning delta (Gemini thought flag pattern).
	if choice.Delta.Content != "" || choice.Delta.ThoughtSignature != "" {
		if choice.Delta.Thought != nil && *choice.Delta.Thought {
			ch <- aikit.StreamEvent{
				Type:             aikit.StreamEventReasoningDelta,
				TextDelta:        choice.Delta.Content,
				ThoughtSignature: choice.Delta.ThoughtSignature,
			}
		} else if choice.Delta.Content != "" {
			ch <- aikit.StreamEvent{
				Type:             aikit.StreamEventTextDelta,
				TextDelta:        choice.Delta.Content,
				ThoughtSignature: choice.Delta.ThoughtSignature,
			}
		}
	}

	// Tool call argument deltas.
	for _, tc := range choice.Delta.ToolCalls {
		var sig string
		if tc.ExtraContent != nil {
			sig = tc.ExtraContent.Google.ThoughtSignature
		}
		ch <- aikit.StreamEvent{
			Type:              aikit.StreamEventToolCallDelta,
			ToolCallIndex:     tc.Index,
			ToolCallID:        tc.ID,
			ToolCallName:      tc.Function.Name,
			ToolCallArgsDelta: tc.Function.Arguments,
			ThoughtSignature:  sig,
		}
	}
}
