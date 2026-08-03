package generate

import (
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/tool"
)

type (
	ToolDefinition = aikit.ToolDefinition
	ToolChoice     = aikit.ToolChoice
	ToolExecutor   = tool.Executor
	ToolSet        = tool.Set
)

var (
	// ToolChoiceAuto lets the model decide whether and which tool to call (default).
	ToolChoiceAuto = ToolChoice{Type: "auto"}
	// ToolChoiceNone prevents the model from calling any tool.
	ToolChoiceNone = ToolChoice{Type: "none"}
	// ToolChoiceRequired forces the model to call at least one tool.
	ToolChoiceRequired = ToolChoice{Type: "required"}
)

// ToolChoiceSpecific returns a ToolChoice that forces the model to call toolName.
func ToolChoiceSpecific(toolName string) ToolChoice {
	return ToolChoice{Type: "tool", ToolName: toolName}
}

// Tool-result content kinds and the ToolResult/ToolResultContent types are
// aliases of the shared aikit package (see ai/types.go).
const (
	ToolResultContentTypeText  = aikit.ToolResultContentTypeText
	ToolResultContentTypeFile  = aikit.ToolResultContentTypeFile
	ToolResultContentTypeJSON  = aikit.ToolResultContentTypeJSON
	ToolResultContentTypeImage = aikit.ToolResultContentTypeImage
)

type (
	ToolResultContent = aikit.ToolResultContent
	ToolResult        = aikit.ToolResult
)

type (
	ToolResultStream      = tool.ResultStream
	StreamingToolExecutor = tool.StreamingExecutor
)
