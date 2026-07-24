package ai

import "github.com/open-ai-sdk/ai-go/aitypes"

// StepEvent and its event-kind enum are aliases of the shared aitypes package.
// The engine emits aitypes.StepEvent values; because an alias is the same type,
// StreamResult can hand them to external consumers as ai.StepEvent (or
// aitypes.StepEvent) with no conversion, and consumers can name the type without
// importing anything under internal/.
type (
	StepEvent     = aitypes.StepEvent
	StepEventType = aitypes.StepEventType
)

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
