package ai

import (
	"context"

	"github.com/open-ai-sdk/ai-go/aitypes"
)

// RepairToolCallInput is passed to RepairToolCallFunc.
type RepairToolCallInput struct {
	Instructions string
	Messages     []Message
	ToolCall     ToolCallOutput
	Tools        *ToolSet
	Error        error
}

// RepairToolCallFunc attempts to repair an invalid tool call.
// Returning nil leaves the original invalid tool call behavior unchanged.
type RepairToolCallFunc func(context.Context, RepairToolCallInput) (*ToolCallOutput, error)

// Tool error types are aliases of the shared aitypes definitions (see
// aitypes/tool-errors.go and ai/errors.go).
type (
	NoSuchToolError           = aitypes.NoSuchToolError
	InvalidToolArgumentsError = aitypes.InvalidToolArgumentsError
)
