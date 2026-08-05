package agui

import "github.com/open-ai-sdk/ai-go/aikit"

// AG-UI event names. Only the subset TanStack AI's StreamChunk union carries is
// emitted; TEXT_MESSAGE_CHUNK, TOOL_CALL_CHUNK, ACTIVITY_*, RAW, and the
// deprecated THINKING_* family are outside that union and would be ignored by
// the client, so they are never produced.
const (
	eventRunStarted  = "RUN_STARTED"
	eventRunFinished = "RUN_FINISHED"
	eventRunError    = "RUN_ERROR"

	eventStepStarted  = "STEP_STARTED"
	eventStepFinished = "STEP_FINISHED"

	eventTextMessageStart   = "TEXT_MESSAGE_START"
	eventTextMessageContent = "TEXT_MESSAGE_CONTENT"
	eventTextMessageEnd     = "TEXT_MESSAGE_END"

	eventReasoningStart          = "REASONING_START"
	eventReasoningMessageStart   = "REASONING_MESSAGE_START"
	eventReasoningMessageContent = "REASONING_MESSAGE_CONTENT"
	eventReasoningMessageEnd     = "REASONING_MESSAGE_END"
	eventReasoningEnd            = "REASONING_END"

	eventToolCallStart  = "TOOL_CALL_START"
	eventToolCallArgs   = "TOOL_CALL_ARGS"
	eventToolCallEnd    = "TOOL_CALL_END"
	eventToolCallResult = "TOOL_CALL_RESULT"

	eventStateSnapshot    = "STATE_SNAPSHOT"
	eventStateDelta       = "STATE_DELTA"
	eventMessagesSnapshot = "MESSAGES_SNAPSHOT"

	// STATE_* is the one family outside TanStack's own message model: its stream
	// processor has no handler for either event, so neither reaches a message
	// part. They are delivered to the application through useChat's onChunk
	// callback, which runs on every chunk before the processor sees it. Both are
	// produced only from the matching engine events — never synthesized here —
	// so a run that publishes no state writes no state frames.

	eventCustom = "CUSTOM"
)

// messagesExtraKey carries the request's verbatim message array from the
// decoder to the encoder, so an interrupt can republish the whole conversation.
const messagesExtraKey = "aguiRequestMessages"

// Custom event names. The structured-output name matches TanStack AI's own
// KnownCustomEvent catalog so existing clients narrow chunk.value without a cast.
const (
	customSourceName           = "source"
	customFileName             = "file"
	customStructuredOutputName = "structured-output.complete"
	// The client routes assistant text into a structured-output part, and
	// exposes a progressively parsed partial, only for messages it saw
	// announced by this event. See WithStructuredOutputStart.
	customStructuredOutputStartName = "structured-output.start"
)

// Finish reasons accepted by TanStack AI's RunFinishedEvent. AG-UI itself has no
// finishReason field; the value is a documented TanStack extension carried
// through @ag-ui/core's passthrough schema.
const (
	finishReasonStop          = "stop"
	finishReasonLength        = "length"
	finishReasonContentFilter = "content_filter"
	finishReasonToolCalls     = "tool_calls"
)

// wireFinishReason maps an engine finish reason onto the TanStack vocabulary.
// An unmapped reason yields "" so the field is omitted rather than guessed.
func wireFinishReason(reason aikit.FinishReason) string {
	switch reason {
	case aikit.FinishReasonStop:
		return finishReasonStop
	case aikit.FinishReasonLength:
		return finishReasonLength
	case aikit.FinishReasonContentFilter:
		return finishReasonContentFilter
	case aikit.FinishReasonToolCalls:
		return finishReasonToolCalls
	default:
		return ""
	}
}

// usagePayload converts engine usage into TanStack AI's TokenUsage shape. AG-UI
// carries no usage of its own, and the field names differ from aikit's, so the
// translation is explicit rather than a struct tag.
func usagePayload(usage aikit.Usage) map[string]any {
	payload := map[string]any{
		"promptTokens":     usage.InputTokens,
		"completionTokens": usage.OutputTokens,
		"totalTokens":      usage.TotalTokens,
	}
	if details := inputTokenDetails(usage); len(details) > 0 {
		payload["promptTokensDetails"] = details
	}
	if usage.OutputTokenDetails.ReasoningTokens != 0 {
		payload["completionTokensDetails"] = map[string]any{
			"reasoningTokens": usage.OutputTokenDetails.ReasoningTokens,
		}
	}
	return payload
}

func inputTokenDetails(usage aikit.Usage) map[string]any {
	details := map[string]any{}
	if usage.InputTokenDetails.CacheReadTokens != 0 {
		details["cachedTokens"] = usage.InputTokenDetails.CacheReadTokens
	}
	if usage.InputTokenDetails.CacheWriteTokens != 0 {
		details["cacheWriteTokens"] = usage.InputTokenDetails.CacheWriteTokens
	}
	return details
}
