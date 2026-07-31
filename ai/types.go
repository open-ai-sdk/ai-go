package ai

import "github.com/open-ai-sdk/ai-go/aikit"

// StepEvent and its event-kind enum are aliases of the shared aikit package.
// The engine emits aikit.StepEvent values; because an alias is the same type,
// StreamResult can hand them to external consumers as ai.StepEvent (or
// aikit.StepEvent) with no conversion, and consumers can name the type without
// importing anything under internal/.
type (
	StepEvent     = aikit.StepEvent
	StepEventType = aikit.StepEventType
)

const (
	StepEventTextDelta           = aikit.StepEventTextDelta
	StepEventReasoningDelta      = aikit.StepEventReasoningDelta
	StepEventToolCallStart       = aikit.StepEventToolCallStart
	StepEventToolCallDelta       = aikit.StepEventToolCallDelta
	StepEventToolCallReady       = aikit.StepEventToolCallReady
	StepEventToolResult          = aikit.StepEventToolResult
	StepEventToolApprovalRequest = aikit.StepEventToolApprovalRequest
	StepEventToolOutputDenied    = aikit.StepEventToolOutputDenied
	StepEventUsage               = aikit.StepEventUsage
	StepEventStepStart           = aikit.StepEventStepStart
	StepEventStepEnd             = aikit.StepEventStepEnd
	StepEventToolCallInvalid     = aikit.StepEventToolCallInvalid
	StepEventStructuredOutput    = aikit.StepEventStructuredOutput
	StepEventDone                = aikit.StepEventDone
	StepEventError               = aikit.StepEventError
	StepEventSource              = aikit.StepEventSource
	StepEventFileDelta           = aikit.StepEventFileDelta
)
