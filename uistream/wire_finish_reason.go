package uistream

import "github.com/open-ai-sdk/ai-go/aitypes"

// wireFinishReason maps the SDK's internal finish-reason vocabulary to the
// values accepted by the AI SDK UI message stream protocol.
func wireFinishReason(reason aitypes.FinishReason) (string, bool) {
	switch reason {
	case aitypes.FinishReasonStop:
		return "stop", true
	case aitypes.FinishReasonToolCalls:
		return "tool-calls", true
	case aitypes.FinishReasonLength:
		return "length", true
	case aitypes.FinishReasonContentFilter:
		return "content-filter", true
	case aitypes.FinishReasonError:
		return "error", true
	case aitypes.FinishReasonUnknown:
		return "other", true
	default:
		return "", false
	}
}
