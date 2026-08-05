package aikit

import "encoding/json"

// StepEventType identifies the kind of event emitted by the engine during a run.
type StepEventType int

const (
	StepEventTextDelta StepEventType = iota
	StepEventReasoningDelta
	StepEventToolCallStart // first delta for a new tool call
	StepEventToolCallDelta // subsequent argument fragment
	StepEventToolCallReady // tool call complete, about to execute
	StepEventToolResult    // tool execution result
	StepEventToolApprovalRequest
	StepEventToolOutputDenied
	StepEventUsage
	StepEventStepStart
	StepEventStepEnd
	StepEventToolCallInvalid // tool call had invalid JSON args, skipped execution
	StepEventStructuredOutput
	StepEventDone
	StepEventError
	// StepEventSource carries a source reference from a provider-native tool.
	StepEventSource
	// StepEventFileDelta carries a file/image output part from the model.
	StepEventFileDelta
	// StepEventClientToolRequest reports a call to a tool declared as
	// client-executed. The turn ends after it; the UI runs the tool and returns
	// the result in the next request's history. Appended last so the existing
	// iota-derived values stay stable.
	StepEventClientToolRequest
	// StepEventStateSnapshot carries the complete run state as a JSON document.
	StepEventStateSnapshot
	// StepEventStateDelta carries an RFC-6902 patch against the last published
	// state. The engine never applies it; it is forwarded to the UI verbatim.
	StepEventStateDelta
)

// StepEvent is a single event emitted by the engine's Run goroutine.
type StepEvent struct {
	Type StepEventType
	// MessageID is the provider's assistant-message ID on terminal events. It
	// is never a tool-call ID.
	MessageID string

	// Text/reasoning fields.
	TextDelta      string
	ReasoningDelta string

	// Tool call fields.
	ToolCallIndex     int
	ToolCallID        string
	ToolCallName      string
	ToolCallArgsDelta string
	ThoughtSignature  string

	// Approval fields (set for StepEventToolApprovalRequest). ApprovalIsAutomatic
	// marks an approval that was granted without human input; ApprovalSignature
	// carries the runtime HMAC signature that must be echoed on stateless resume.
	// Both are optional and omitted from the UI chunk when unset.
	ApprovalIsAutomatic bool
	ApprovalID          string
	ApprovalSignature   string

	// Tool result.
	ToolResult *ToolResult

	// Usage is the latest cumulative usage snapshot for the current step, not
	// an additive delta.
	Usage *Usage

	// Step metadata.
	StepNumber   int
	FinishReason FinishReason
	// RawFinishReason is the unmodified finish reason string from the provider.
	RawFinishReason string
	// ProviderMetadata carries provider-specific metadata.
	ProviderMetadata map[string]any
	// Warnings carries non-fatal advisories.
	Warnings []Warning

	// Source is set for StepEventSource events.
	Source *Source

	// File fields are set for StepEventFileDelta.
	FileData      []byte
	FileMediaType string

	// Structured output (final step only).
	StructuredOutput json.RawMessage

	// State is the full state document for StepEventStateSnapshot. It is
	// forwarded to the browser verbatim, so it must never carry secrets, API
	// keys, or raw provider responses.
	State json.RawMessage
	// StatePatch is the RFC-6902 operation array for StepEventStateDelta. The
	// engine validates its shape but never applies it.
	StatePatch json.RawMessage

	// Error.
	Error error
}
