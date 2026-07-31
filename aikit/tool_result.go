package aikit

const (
	ToolResultContentTypeText = "text"
	ToolResultContentTypeFile = "file"
)

// ToolResultContent represents a single content part in a tool result.
type ToolResultContent struct {
	Type      string
	Text      string
	Data      []byte
	MediaType string
}

// ToolResult holds the output of a single tool invocation.
type ToolResult struct {
	ID     string
	Name   string
	Args   string
	Output string
	// Error retains typed tool failure classification for agent and wire
	// adapters. Output remains the model-visible raw string.
	Error   error `json:"-"`
	Content []ToolResultContent
}
