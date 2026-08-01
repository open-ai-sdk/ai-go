package uistream

import "github.com/open-ai-sdk/ai-go/aikit"

// wireFinishReason maps the SDK's internal finish-reason vocabulary to the
// values accepted by the AI SDK UI message stream protocol.
func wireFinishReason(reason aikit.FinishReason) (string, bool) {
	switch reason {
	case aikit.FinishReasonStop:
		return "stop", true
	case aikit.FinishReasonToolCalls:
		return "tool-calls", true
	case aikit.FinishReasonLength:
		return "length", true
	case aikit.FinishReasonContentFilter:
		return "content-filter", true
	case aikit.FinishReasonError:
		return "error", true
	case aikit.FinishReasonUnknown:
		return "other", true
	default:
		return "", false
	}
}
