// Package aitypes holds the shared, public data types that flow between the ai
// package, the internal tool-loop engine, and the uistream package. It is a leaf
// package: it imports only the standard library and is imported by ai, engine,
// and uistream so those packages name one identical set of types rather than
// maintaining duplicate universes bridged by converters.
//
// Everything here is public API — keep it minimal and stable. Types only, no
// behavior: adding a field is additive, removing or reshaping one is breaking.
package aitypes

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
)

// StepEvent is a single event emitted by the engine's Run goroutine.
type StepEvent struct {
	Type StepEventType

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
	// carries a provider-supplied signature for the approval. Both are optional
	// and omitted from the UI chunk when unset.
	ApprovalIsAutomatic bool
	ApprovalSignature   string

	// Tool result.
	ToolResult *ToolResult

	// Usage.
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

	// Error.
	Error error
}
