package openaicompat

import (
	"strings"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// MapFinishReason converts a raw Chat Completions finish reason.
func MapFinishReason(s string) aikit.FinishReason {
	switch strings.ToLower(s) {
	case "stop":
		return aikit.FinishReasonStop
	case "tool_calls":
		return aikit.FinishReasonToolCalls
	case "length":
		return aikit.FinishReasonLength
	case "content_filter":
		return aikit.FinishReasonContentFilter
	default:
		return aikit.FinishReasonUnknown
	}
}
