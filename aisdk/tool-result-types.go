package aisdk

// Promoted from aitypes/tool-and-stream-types.go. Only the tool-result and source
// shapes came across: they are reachable from StepEvent and therefore part of the
// protocol vocabulary. The provider-stream half of that file (StreamEventType,
// StreamEvent, Warning) described the deleted engine's provider abstraction and had
// zero references here, so it did not survive.

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

// StepToolResult holds the output of a single tool invocation as it appears on a
// StepEvent.
//
// Named for its scope rather than promoted as bare ToolResult: this package already
// exports a ToolResult, the public payload of ToolResultHook, whose fields are the
// wire-facing ToolCallID/ToolName/ArgsJSON. Two different shapes for two different
// jobs, so they get two different names.
type StepToolResult struct {
	ID      string
	Name    string
	Args    string
	Output  string
	Content []ToolResultContent // optional multi-part content
}

// Source represents a single source reference from a provider-native tool.
//
// This is the superset of the shape the SSE writer previously declared locally
// (ID/URL/Title); WriteSource reads only those three, so one type serves both the
// StepEvent field and the writer.
type Source struct {
	SourceType       string
	ID               string
	URL              string
	Title            string
	ProviderMetadata map[string]any
}

// Warning is a non-fatal advisory from a provider, carried on StepEvent.Warnings.
type Warning struct {
	Type    string
	Message string
	Setting string
}
