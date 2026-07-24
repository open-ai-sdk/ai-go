// Package engine implements the multi-step tool-loop that drives GenerateText and StreamText.
// It defines its own model interface and event types to avoid import cycles with package ai.
package engine

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

// FinishReason indicates why the model stopped generating.
type FinishReason string

const (
	FinishReasonStop          FinishReason = "stop"
	FinishReasonToolCalls     FinishReason = "tool_calls"
	FinishReasonLength        FinishReason = "length"
	FinishReasonContentFilter FinishReason = "content_filter"
	FinishReasonError         FinishReason = "error"
	FinishReasonUnknown       FinishReason = "unknown"
)

// Usage holds token counts for a completion step.
type Usage struct {
	InputTokens        int
	InputTokenDetails  InputTokenDetails
	OutputTokens       int
	OutputTokenDetails OutputTokenDetails
	TotalTokens        int
	Raw                map[string]any
}

type InputTokenDetails struct {
	NoCacheTokens    int
	CacheReadTokens  int
	CacheWriteTokens int
}

type OutputTokenDetails struct {
	TextTokens      int
	ReasoningTokens int
}

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

// Tool-result content part kinds. Images, audio and documents all travel as a
// single file part carrying a media type — there is no separate image kind.
const (
	ToolResultContentTypeText = "text"
	ToolResultContentTypeFile = "file"
)

// ToolResultContent represents a single content part in a tool result.
type ToolResultContent struct {
	Type string // ToolResultContentTypeText or ToolResultContentTypeFile
	Text string // for type="text"
	Data []byte // for type="file"
	// MediaType is either a full IANA media type ("image/png") or just the
	// top-level segment ("image"); providers narrow it as their API requires.
	MediaType string // for type="file"
}

// ToolResult holds the output of a single tool invocation.
type ToolResult struct {
	ID      string
	Name    string
	Args    string
	Output  string
	Content []ToolResultContent // optional multi-part content
}

// StreamEventType identifies provider stream event kinds.
type StreamEventType int

const (
	StreamEventTextDelta StreamEventType = iota
	StreamEventReasoningDelta
	StreamEventToolCallDelta
	StreamEventUsage
	StreamEventFinish
	StreamEventError
	// StreamEventSource carries a source reference (web search result, document, etc.)
	StreamEventSource
	// StreamEventFileDelta carries a file/image output part from the model.
	StreamEventFileDelta
)

// Source represents a single source reference from a provider-native tool.
type Source struct {
	SourceType       string
	ID               string
	URL              string
	Title            string
	ProviderMetadata map[string]any
}

// Warning is a non-fatal advisory from a provider.
type Warning struct {
	Type    string
	Message string
	Setting string
}

// StreamEvent is a normalized event from a Model stream.
type StreamEvent struct {
	Type StreamEventType

	TextDelta         string
	ToolCallIndex     int
	ToolCallID        string
	ToolCallName      string
	ToolCallArgsDelta string
	ThoughtSignature  string
	Usage             *Usage
	FinishReason      FinishReason
	// RawFinishReason is the unmodified finish reason string from the provider.
	RawFinishReason string
	// ProviderMetadata carries provider-specific metadata.
	ProviderMetadata map[string]any
	// Warnings carries non-fatal advisories from the provider.
	Warnings []Warning
	// Source is set for StreamEventSource events.
	Source *Source
	// File fields are set for StreamEventFileDelta.
	FileData      []byte
	FileMediaType string
	Error         error
}
