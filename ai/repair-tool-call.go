package ai

import (
	"context"
	"fmt"
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

// NoSuchToolError indicates that the model called a tool that is not active.
type NoSuchToolError struct {
	ToolName       string
	AvailableTools []string
}

func (e *NoSuchToolError) Error() string {
	if len(e.AvailableTools) == 0 {
		return fmt.Sprintf("unknown tool %q", e.ToolName)
	}
	return fmt.Sprintf("unknown tool %q (available: %v)", e.ToolName, e.AvailableTools)
}

// InvalidToolArgumentsError indicates that the tool arguments were not valid JSON.
type InvalidToolArgumentsError struct {
	ToolName string
	Args     string
	Cause    error
}

func (e *InvalidToolArgumentsError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("invalid arguments for tool %q", e.ToolName)
}

func (e *InvalidToolArgumentsError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
