package aikit

import (
	"time"
)

// ToolDefinition describes a callable function tool available to a model.
type ToolDefinition struct {
	Name          string
	Description   string
	InputSchema   map[string]any
	ContextSchema map[string]any
	ToModelOutput func(result string) string
	Timeout       time.Duration
}

// ToolChoice controls which tool a model may call.
type ToolChoice struct {
	Type     string
	ToolName string
}
