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
	Usage *StreamUsage `json:"usage"`
	// ID and Model are the response id and the model that answered, repeated on
	// every chunk by OpenAI-style endpoints.
	ID    string `json:"id,omitempty"`
	Model string `json:"model,omitempty"`
	// Error is set when the endpoint reports a failure inside the event stream
	// (for example OpenRouter mid-stream errors) instead of an HTTP status.
	Error *StreamError `json:"error,omitempty"`
	// ProviderMetadata holds provider-specific metadata from the response (e.g. Gemini groundingMetadata).
	ProviderMetadata map[string]any `json:"provider_metadata,omitempty"`
}

// StreamUsage is the usage object of a stream chunk. Raw keeps the complete
// provider-reported object, including fields this package does not model
// (for example OpenRouter's cost and cost_details).
type StreamUsage struct {
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

	// Raw is the full decoded usage object.
	Raw map[string]any `json:"-"`
}

// UnmarshalJSON decodes the typed fields and retains the raw object.
func (u *StreamUsage) UnmarshalJSON(data []byte) error {
	type plain StreamUsage
	var typed plain
	if err := json.Unmarshal(data, &typed); err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	typed.Raw = raw
	*u = StreamUsage(typed)
	return nil
}

// StreamError is an error object reported inside an SSE data event.
type StreamError struct {
	Message string `json:"message"`
	Type    string `json:"type,omitempty"`
	Code    any    `json:"code,omitempty"`
}

func (e *StreamError) describe() string {
	switch {
	case e.Message != "" && e.Code != nil:
		return fmt.Sprintf("%s (code %v)", e.Message, e.Code)
	case e.Message != "":
		return e.Message
	case e.Code != nil:
		return fmt.Sprintf("code %v", e.Code)
	}
	return "unspecified stream error"
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
	var identity responseIdentity
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
					Type:             aikit.StreamEventFinish,
					FinishReason:     aikit.FinishReasonStop,
					RawFinishReason:  "stop",
					Warnings:         params.EncodeWarnings,
					ProviderMetadata: mergeIdentity(nil, providerKey(params), identity),
				}
			}
			return nil
		}

		var chunk StreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("%s: unmarshal chunk: %w", providerName, err)
		}

		if chunk.Error != nil {
			return fmt.Errorf("%s: stream error: %s", providerName, chunk.Error.describe())
		}
		identity.observe(chunk)
		emitChunkEvents(chunk, ch, params, &finishEmitted, identity)
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
			Raw:         usageRaw(u, cachedTokens, reasoningTokens),
		},
	}
}

// responseIdentity remembers the response id and answering model seen on any
// chunk so the finish event can report them.
type responseIdentity struct{ id, model string }

func (r *responseIdentity) observe(chunk StreamChunk) {
	if chunk.ID != "" {
		r.id = chunk.ID
	}
	if chunk.Model != "" {
		r.model = chunk.Model
	}
}

// mergeIdentity adds responseId and model under the provider's metadata key
// without overwriting values an extractor already set.
func mergeIdentity(meta map[string]any, provider string, id responseIdentity) map[string]any {
	if id.id == "" && id.model == "" {
		return meta
	}
	if meta == nil {
		meta = map[string]any{}
	}
	entry, ok := meta[provider].(map[string]any)
	if !ok || entry == nil {
		entry = map[string]any{}
	}
	if _, set := entry["responseId"]; !set && id.id != "" {
		entry["responseId"] = id.id
	}
	if _, set := entry["model"]; !set && id.model != "" {
		entry["model"] = id.model
	}
	meta[provider] = entry
	return meta
}

// usageRaw returns the provider's full usage object plus the normalized
// cached_tokens and reasoning_tokens counters.
func usageRaw(u *StreamUsage, cachedTokens, reasoningTokens int) map[string]any {
	raw := make(map[string]any, len(u.Raw)+5)
	for k, v := range u.Raw {
		raw[k] = v
	}
	raw["prompt_tokens"] = u.PromptTokens
	raw["completion_tokens"] = u.CompletionTokens
	raw["total_tokens"] = u.TotalTokens
	raw["cached_tokens"] = cachedTokens
	raw["reasoning_tokens"] = reasoningTokens
	return raw
}

func providerKey(params SSEDecodeParams) string {
	if params.ProviderName == "" {
		return "openaichat"
	}
	return params.ProviderName
}

func emitChunkEvents(
	chunk StreamChunk,
	ch chan<- aikit.StreamEvent,
	params SSEDecodeParams,
	finishEmitted *bool,
	identity responseIdentity,
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
		meta = mergeIdentity(meta, providerKey(params), identity)
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
