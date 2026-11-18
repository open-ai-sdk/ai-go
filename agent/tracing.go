package agent

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/open-ai-sdk/ai-go/internal/tracing"
)

// stepAttrs reports the finish reason and token usage recorded for one model
// call. It never includes prompt or completion text — that is content,
// attached separately and only when the caller opted into WithTraceContent.
func stepAttrs(sr streamResult) []tracing.Attr {
	attrs := []tracing.Attr{{Key: "ai.finish_reason", Value: string(sr.finish)}}
	if sr.usage != nil {
		attrs = append(attrs,
			tracing.Attr{Key: "ai.usage.input_tokens", Value: sr.usage.InputTokens},
			tracing.Attr{Key: "ai.usage.output_tokens", Value: sr.usage.OutputTokens},
			tracing.Attr{Key: "ai.usage.total_tokens", Value: sr.usage.TotalTokens},
		)
	}
	return attrs
}

// Attribute names follow the OpenTelemetry generative-AI semantic conventions.
// Cache reads, cache writes and reasoning tokens use the dedicated usage keys
// the conventions define for them; every key is emitted only when the provider
// reported a value.
const (
	attrGenAIRequestModel    = "gen_ai.request.model"
	attrGenAIResponseModel   = "gen_ai.response.model"
	attrGenAIResponseID      = "gen_ai.response.id"
	attrGenAIFinishReasons   = "gen_ai.response.finish_reasons"
	attrGenAIUsageInput      = "gen_ai.usage.input_tokens"
	attrGenAIUsageOutput     = "gen_ai.usage.output_tokens"
	attrGenAIUsageCacheRead  = "gen_ai.usage.cache_read.input_tokens"
	attrGenAIUsageCacheWrite = "gen_ai.usage.cache_creation.input_tokens"
	attrGenAIUsageReasoning  = "gen_ai.usage.reasoning.output_tokens"
)

// modelCallAttrs reports the gen_ai.* attributes for one finished model call,
// next to (not instead of) the ai.* attributes from [stepAttrs]. The response
// id and model come from the finish event's provider metadata: any provider
// entry (or the top level) carrying string "responseId" / "model" keys is
// used, with provider names visited in sorted order so the result is
// deterministic. The finish reason is the provider's raw value when it
// reported one, otherwise the normalized reason. It never includes content.
func modelCallAttrs(sr streamResult) []tracing.Attr {
	attrs := make([]tracing.Attr, 0, 8)
	if id := providerMetadataString(sr.providerMeta, "responseId"); id != "" {
		attrs = append(attrs, tracing.Attr{Key: attrGenAIResponseID, Value: id})
	}
	if model := providerMetadataString(sr.providerMeta, "model"); model != "" {
		attrs = append(attrs, tracing.Attr{Key: attrGenAIResponseModel, Value: model})
	}
	if reason := sr.rawFinish; reason != "" || sr.finish != "" {
		if reason == "" {
			reason = string(sr.finish)
		}
		attrs = append(attrs, tracing.Attr{Key: attrGenAIFinishReasons, Value: []string{reason}})
	}
	if sr.usage != nil {
		attrs = append(attrs,
			tracing.Attr{Key: attrGenAIUsageInput, Value: sr.usage.InputTokens},
			tracing.Attr{Key: attrGenAIUsageOutput, Value: sr.usage.OutputTokens},
		)
		attrs = appendPositive(attrs, attrGenAIUsageCacheRead, sr.usage.InputTokenDetails.CacheReadTokens)
		attrs = appendPositive(attrs, attrGenAIUsageCacheWrite, sr.usage.InputTokenDetails.CacheWriteTokens)
		attrs = appendPositive(attrs, attrGenAIUsageReasoning, sr.usage.OutputTokenDetails.ReasoningTokens)
	}
	return attrs
}

func appendPositive(attrs []tracing.Attr, key string, value int) []tracing.Attr {
	if value > 0 {
		attrs = append(attrs, tracing.Attr{Key: key, Value: value})
	}
	return attrs
}

// providerMetadataString finds key as a non-empty string at the top level of
// meta or inside any provider's metadata map.
func providerMetadataString(meta map[string]any, key string) string {
	if value, ok := meta[key].(string); ok && value != "" {
		return value
	}
	providers := make([]string, 0, len(meta))
	for name := range meta {
		providers = append(providers, name)
	}
	sort.Strings(providers)
	for _, name := range providers {
		switch nested := meta[name].(type) {
		case map[string]any:
			if value, ok := nested[key].(string); ok && value != "" {
				return value
			}
		case map[string]string:
			if value := nested[key]; value != "" {
				return value
			}
		}
	}
	return ""
}

// marshalMessagesForTrace renders the conversation sent to the model as a
// span attribute value. Only called by sites gated on traceContent — the
// span content policy is enforced by those call sites, not by this helper.
func marshalMessagesForTrace(msgs []Message) string {
	b, err := json.Marshal(msgs)
	if err != nil {
		return fmt.Sprintf("<unmarshalable: %v>", err)
	}
	return string(b)
}
