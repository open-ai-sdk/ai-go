package aitypes

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
