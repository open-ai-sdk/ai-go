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
	// ClientExecuted marks a tool this process declares but never runs. The
	// model sees it like any other tool; the runtime streams the call and then
	// suspends the turn so the caller's UI can execute it and return the result
	// in the next request's history. Its output is therefore untrusted input.
	ClientExecuted bool
}

// ToolChoice controls which tool a model may call.
type ToolChoice struct {
	Type     string
	ToolName string
}
