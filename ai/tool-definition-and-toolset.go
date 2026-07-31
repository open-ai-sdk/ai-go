package ai

import (
	"context"

	"github.com/open-ai-sdk/ai-go/aikit"
)

type (
	ToolDefinition = aikit.ToolDefinition
	ToolChoice     = aikit.ToolChoice
	ToolExecutor   = aikit.ToolExecutor
	ToolSet        = aikit.ToolSet
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
	ToolResultContentTypeText = aikit.ToolResultContentTypeText
	ToolResultContentTypeFile = aikit.ToolResultContentTypeFile
)

type (
	ToolResultContent = aikit.ToolResultContent
	ToolResult        = aikit.ToolResult
)

// ToolResultStream allows tools to stream partial output to the UI in real-time.
type ToolResultStream interface {
	// Write sends a partial result to the UI (e.g., stdout from a bash command).
	Write(partial string)
}

// StreamingToolExecutor extends ToolExecutor with streaming support.
// Tools that implement this interface receive a stream for real-time output.
type StreamingToolExecutor interface {
	ToolExecutor
	// ExecuteStreaming executes a tool with a stream for partial results.
	// Falls back to Execute if not implemented.
	ExecuteStreaming(ctx context.Context, name, argsJSON string, stream ToolResultStream) (string, error)
}
