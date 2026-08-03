package aikit

// StreamEventType identifies provider stream event kinds.
type StreamEventType int

const (
	StreamEventTextDelta StreamEventType = iota
	StreamEventReasoningDelta
	StreamEventToolCallDelta
	StreamEventUsage
	StreamEventFinish
	StreamEventError
	StreamEventSource
	StreamEventFileDelta
)

// Source represents a provider-native source reference.
type Source struct {
	SourceType       string
	ID               string
	URL              string
	Title            string
	ProviderMetadata map[string]any
}

// StreamEvent is a normalized event from a [Model] stream.
type StreamEvent struct {
	Type StreamEventType
	// MessageID is the provider's assistant-message ID on terminal events. It
	// is never a tool-call ID.
	MessageID string

	TextDelta         string
	ToolCallIndex     int
	ToolCallID        string
	ToolCallName      string
	ToolCallArgsDelta string
	ThoughtSignature  string
	Usage             *Usage
	FinishReason      FinishReason
	RawFinishReason   string
	ProviderMetadata  map[string]any
	Warnings          []Warning
	Source            *Source
	FileData          []byte
	FileMediaType     string
	Error             error
}
