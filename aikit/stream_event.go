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
