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

// PrepareStepContext provides information about the current tool-loop step.
type PrepareStepContext struct {
	StepNumber     int
	Steps          []PrepareStepInfo
	ToolsContext   ToolsContext
	RuntimeContext RuntimeContext
}

// PrepareStepInfo describes a completed step available to PrepareStep.
type PrepareStepInfo struct {
	StepNumber       int
	HasToolCalls     bool
	ToolNames        []string
	Text             string
	Reasoning        string
	ToolCalls        []ToolCallInfo
	ToolResults      []ToolResult
	Usage            *Usage
	FinishReason     FinishReason
	RawFinishReason  string
	ProviderMetadata map[string]any
	Warnings         []Warning
}

// PrepareStepResult holds per-step request overrides.
type PrepareStepResult struct {
	Model           Model
	ToolChoice      *ToolChoice
	ActiveTools     []string
	Instructions    string
	ProviderOptions map[string]any
}

// PrepareStepFunc is called before each tool-loop step.
type PrepareStepFunc func(PrepareStepContext) *PrepareStepResult

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
