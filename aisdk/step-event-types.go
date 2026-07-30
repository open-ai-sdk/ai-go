// StepEvent and friends are the normalized event vocabulary the chunk producer
// consumes. Promoted here from the former leaf package aitypes when the tool-loop
// engine and provider layer were deleted: with only one consumer left there is no
// longer a cycle to break, and keeping a separate package would have meant two
// vocabularies bridged by converters — the exact thing aitypes existed to avoid.
//
// The Eino adapter extends this rather than defining a parallel event type.
package aisdk

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

	// Block-lifecycle variants, added for the Eino adapter.
	//
	// The v7 wire requires text-start before any text-delta and reasoning-start before
	// any reasoning-delta — the client throws otherwise — and each carries a block id
	// that must be unique across the whole message, not just the current model turn.
	// Before these existed the producer derived ids internally, which works only when
	// the producer is also the thing that knows where a block begins. A block-structured
	// source like Eino knows that already, so it says so rather than having it guessed.
	//
	// Appended rather than inserted: these are iota values and the producer switches on
	// them, so renumbering the existing set would be a silent behaviour change.
	StepEventRunStart
	StepEventTextStart
	StepEventTextEnd
	StepEventReasoningStart
	StepEventReasoningEnd
	StepEventToolError
	StepEventRunFinish
	// StepEventInterrupted carries an agent interrupt — the approval gate's signal that
	// a tool is waiting on a human. Opaque here on purpose: aisdk must not name Eino
	// types, so the adapter passes the interrupt through as `any`.
	StepEventInterrupted
)

// String makes failures in table-driven tests readable, since these are otherwise bare
// integers in diff output.
func (t StepEventType) String() string {
	if name, ok := stepEventNames[t]; ok {
		return name
	}
	return "StepEvent(?)"
}

var stepEventNames = map[StepEventType]string{
	StepEventTextDelta: "text-delta", StepEventReasoningDelta: "reasoning-delta",
	StepEventToolCallStart: "tool-call-start", StepEventToolCallDelta: "tool-call-delta",
	StepEventToolCallReady: "tool-call-ready", StepEventToolResult: "tool-result",
	StepEventToolApprovalRequest: "tool-approval-request",
	StepEventToolOutputDenied:    "tool-output-denied",
	StepEventUsage:               "usage",
	StepEventStepStart:           "step-start", StepEventStepEnd: "step-end",
	StepEventToolCallInvalid:  "tool-call-invalid",
	StepEventStructuredOutput: "structured-output",
	StepEventDone:             "done", StepEventError: "error",
	StepEventSource: "source", StepEventFileDelta: "file-delta",
	StepEventRunStart: "run-start", StepEventTextStart: "text-start",
	StepEventTextEnd: "text-end", StepEventReasoningStart: "reasoning-start",
	StepEventReasoningEnd: "reasoning-end", StepEventToolError: "tool-error",
	StepEventRunFinish: "run-finish", StepEventInterrupted: "interrupted",
}

// StepEvent is a single event emitted by the engine's Run goroutine.
type StepEvent struct {
	Type StepEventType

	// BlockID identifies the text or reasoning block this event belongs to, and is the
	// `id` the v7 wire carries on text-*/reasoning-* chunks.
	//
	// It must be unique across the whole message. Eino's StreamingMeta.Index is scoped to
	// one model response and — measured in the Phase 00 spike — restarts at 0 on the next
	// turn, so the index alone would collide across a tool boundary. The adapter composes
	// turn and index.
	BlockID string

	// Text/reasoning fields.
	TextDelta      string
	ReasoningDelta string

	// MessageID is set on StepEventRunStart when the source knows the id the assistant
	// message should carry.
	MessageID string

	// ProviderExecuted marks a tool the model provider ran itself rather than one this
	// server dispatched. A pointer because absent and false are different on the wire:
	// absent means "not stated", false means "definitely ours".
	ProviderExecuted *bool

	// ToolInput is the fully-assembled input on StepEventToolCallReady. It is the
	// concatenation of the ToolCallArgsDelta fragments for a streamed call, or the whole
	// value for a provider-executed one whose arguments never stream.
	ToolInput any

	// FileURL is set on StepEventFileDelta when the source gives a URL or data URL rather
	// than raw bytes. FileIsReasoning routes the file to reasoning-file instead of file.
	FileURL         string
	FileIsReasoning bool

	// Interrupt carries an agent interrupt on StepEventInterrupted. Typed as any so this
	// package stays free of any agent-framework import; the adapter and the approval gate
	// both know the concrete type.
	Interrupt any

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
	ToolResult *StepToolResult

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
