// Package engine implements the multi-step tool-loop that drives GenerateText and StreamText.
// Its event and data types are aliases of the public aitypes package, so the ai
// and uistream packages name one identical set of types instead of bridging
// duplicate universes with converters.
package engine

import "github.com/open-ai-sdk/ai-go/aitypes"

// Event and data types are aliases of aitypes: an alias is the same type, so a
// value constructed as engine.StepEvent is also an aitypes.StepEvent and an
// ai.StepEvent with no conversion.
type (
	StepEventType      = aitypes.StepEventType
	StepEvent          = aitypes.StepEvent
	FinishReason       = aitypes.FinishReason
	Usage              = aitypes.Usage
	InputTokenDetails  = aitypes.InputTokenDetails
	OutputTokenDetails = aitypes.OutputTokenDetails
	ToolResultContent  = aitypes.ToolResultContent
	ToolResult         = aitypes.ToolResult
	StreamEventType    = aitypes.StreamEventType
	Source             = aitypes.Source
	Warning            = aitypes.Warning
	StreamEvent        = aitypes.StreamEvent
)

// StepEventType values.
const (
	StepEventTextDelta           = aitypes.StepEventTextDelta
	StepEventReasoningDelta      = aitypes.StepEventReasoningDelta
	StepEventToolCallStart       = aitypes.StepEventToolCallStart
	StepEventToolCallDelta       = aitypes.StepEventToolCallDelta
	StepEventToolCallReady       = aitypes.StepEventToolCallReady
	StepEventToolResult          = aitypes.StepEventToolResult
	StepEventToolApprovalRequest = aitypes.StepEventToolApprovalRequest
	StepEventToolOutputDenied    = aitypes.StepEventToolOutputDenied
	StepEventUsage               = aitypes.StepEventUsage
	StepEventStepStart           = aitypes.StepEventStepStart
	StepEventStepEnd             = aitypes.StepEventStepEnd
	StepEventToolCallInvalid     = aitypes.StepEventToolCallInvalid
	StepEventStructuredOutput    = aitypes.StepEventStructuredOutput
	StepEventDone                = aitypes.StepEventDone
	StepEventError               = aitypes.StepEventError
	StepEventSource              = aitypes.StepEventSource
	StepEventFileDelta           = aitypes.StepEventFileDelta
)

// FinishReason values.
const (
	FinishReasonStop          = aitypes.FinishReasonStop
	FinishReasonToolCalls     = aitypes.FinishReasonToolCalls
	FinishReasonLength        = aitypes.FinishReasonLength
	FinishReasonContentFilter = aitypes.FinishReasonContentFilter
	FinishReasonError         = aitypes.FinishReasonError
	FinishReasonUnknown       = aitypes.FinishReasonUnknown
)

// ToolResultContent kinds.
const (
	ToolResultContentTypeText = aitypes.ToolResultContentTypeText
	ToolResultContentTypeFile = aitypes.ToolResultContentTypeFile
)

// StreamEventType values.
const (
	StreamEventTextDelta      = aitypes.StreamEventTextDelta
	StreamEventReasoningDelta = aitypes.StreamEventReasoningDelta
	StreamEventToolCallDelta  = aitypes.StreamEventToolCallDelta
	StreamEventUsage          = aitypes.StreamEventUsage
	StreamEventFinish         = aitypes.StreamEventFinish
	StreamEventError          = aitypes.StreamEventError
	StreamEventSource         = aitypes.StreamEventSource
	StreamEventFileDelta      = aitypes.StreamEventFileDelta
)

// mergeUsage combines two usage reports from the same step, taking each field
// from the incoming report when it is non-zero and otherwise keeping the prior
// value. This lets providers report token counts across multiple stream events
// without a later partial update zeroing an earlier one.
func mergeUsage(prior, incoming *Usage) *Usage {
	if incoming == nil {
		return prior
	}
	if prior == nil {
		return incoming
	}
	takeInt := func(cur, next int) int {
		if next != 0 {
			return next
		}
		return cur
	}
	merged := *prior
	merged.InputTokens = takeInt(prior.InputTokens, incoming.InputTokens)
	merged.OutputTokens = takeInt(prior.OutputTokens, incoming.OutputTokens)
	merged.TotalTokens = takeInt(prior.TotalTokens, incoming.TotalTokens)
	merged.InputTokenDetails.NoCacheTokens = takeInt(prior.InputTokenDetails.NoCacheTokens, incoming.InputTokenDetails.NoCacheTokens)
	merged.InputTokenDetails.CacheReadTokens = takeInt(prior.InputTokenDetails.CacheReadTokens, incoming.InputTokenDetails.CacheReadTokens)
	merged.InputTokenDetails.CacheWriteTokens = takeInt(prior.InputTokenDetails.CacheWriteTokens, incoming.InputTokenDetails.CacheWriteTokens)
	merged.OutputTokenDetails.TextTokens = takeInt(prior.OutputTokenDetails.TextTokens, incoming.OutputTokenDetails.TextTokens)
	merged.OutputTokenDetails.ReasoningTokens = takeInt(prior.OutputTokenDetails.ReasoningTokens, incoming.OutputTokenDetails.ReasoningTokens)
	if incoming.Raw != nil {
		merged.Raw = incoming.Raw
	}
	return &merged
}
