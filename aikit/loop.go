package aikit

import (
	"context"
	"encoding/json"
)

// StopCondition determines when the tool loop should stop.
type StopCondition func(step int, result *StepResult) bool

// StepResult holds the fields used to evaluate a [StopCondition].
type StepResult struct {
	HasToolCalls bool
	ToolNames    []string
	Text         string
}

// ToolCallInfo describes a completed model tool call.
type ToolCallInfo struct {
	ID               string
	Name             string
	Args             json.RawMessage
	ArgsSet          bool
	ThoughtSignature string
}

// RepairToolCallInput describes a tool call that failed validation.
type RepairToolCallInput struct {
	Instructions string
	Messages     []Message
	ToolCall     ToolCallInfo
	Tools        *ToolSet
	Error        error
}

// RepairToolCallFunc attempts to repair an invalid tool call.
type RepairToolCallFunc func(context.Context, RepairToolCallInput) (*ToolCallInfo, error)
