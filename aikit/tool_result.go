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
	ID      string
	Name    string
	Args    string
	Output  string
	Content []ToolResultContent
}
