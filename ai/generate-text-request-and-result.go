package ai

import "encoding/json"

// GenerateTextRequest is the input to GenerateText and StreamText.
type GenerateTextRequest struct {
	// Model is the language model to call.
	Model LanguageModel
	// Instructions is an optional system prompt prepended before the conversation.
	Instructions string
	// Messages is the conversation history.
	Messages []Message
	// Tools is an optional set of callable functions for multi-step tool loops.
	Tools *ToolSet
	// ToolChoice controls which tool(s) the model may call. Defaults to ToolChoiceAuto.
	// Ignored when Tools is nil.
	ToolChoice *ToolChoice
	// StopWhen is an optional custom stop condition for the tool loop.
	StopWhen StopCondition
	// Output optionally constrains the model's output to a JSON schema or mode.
	Output *OutputSchema
	// Settings controls per-request model parameters (temperature, maxTokens, etc.).
	Settings CallSettings
	// MaxSteps limits the number of tool-loop iterations. Defaults to 10.
	MaxSteps int
	// ProviderOptions carries provider-specific options keyed by provider name.
	// Example: map[string]any{"openai": map[string]any{"previousResponseId": "r_abc"}}.
	ProviderOptions map[string]any
	// PrepareStep is called before each tool-loop step to allow per-step overrides.
	PrepareStep PrepareStepFunc
	// RepairToolCall attempts to repair invalid or unknown tool calls
	// before they are surfaced as invalid.
	RepairToolCall RepairToolCallFunc
	// ActiveTools filters the tool set to only these tool names. Nil means all tools.
	ActiveTools           []string
	ToolsContext          ToolsContext
	RuntimeContext        RuntimeContext
	ToolApproval          map[string]ToolApprovalFunc
	ToolApprovalResponder ToolApprovalResponder
	// OnStepEnd is called after each step completes.
	OnStepEnd func(StepEndEvent)
	// OnEnd is called when the entire run completes.
	OnEnd func(EndEvent)
	// OnChunk is called for every engine event during streaming.
	OnChunk func(ChunkEvent)
	// OnError is called when an error occurs during the run.
	OnError func(error)
	// SmoothStream enables smooth text streaming with configurable chunking.
	// Only used by StreamText; ignored by GenerateText.
	SmoothStream *SmoothStream
	// Middlewares holds deferred model middlewares set via WithMiddleware.
	// Applied after model resolution in Runtime.buildRequest.
	Middlewares []LanguageModelMiddleware
	// ParallelToolExecution enables parallel execution of tool calls within a step.
	ParallelToolExecution bool
	// MaxParallelTools limits concurrent tool executions. Default: 5.
	MaxParallelTools int
}

// StepOutput holds the result of a single tool-loop step.
type StepOutput struct {
	Text             string
	Reasoning        string
	ToolCalls        []ToolCallOutput
	ToolResults      []ToolResult
	Usage            Usage
	FinishReason     FinishReason
	RawFinishReason  string
	ProviderMetadata map[string]any
	Warnings         []Warning
	Sources          []Source
	// Files holds file/image outputs from the model.
	Files []GeneratedFile
	// Response mirrors AI SDK Node's step.response.messages for continuation.
	Response Response
}

// ToolCallOutput holds the details of a single tool call made by the model.
type ToolCallOutput struct {
	ID               string
	Name             string
	Args             json.RawMessage
	ThoughtSignature string
}

// GenerateTextResult holds the full output of a GenerateText call.
type GenerateTextResult struct {
	Text      string
	Reasoning string
	Steps     []StepOutput
	// FinalStep contains the complete final-step output. It is zero when no step completed.
	FinalStep        StepOutput
	ToolResults      []ToolResult
	Usage            Usage
	FinishReason     FinishReason
	RawFinishReason  string
	ProviderMetadata map[string]any
	Warnings         []Warning
	Sources          []Source
	// Files holds file/image outputs from the model (aggregated across all steps).
	Files            []GeneratedFile
	StructuredOutput json.RawMessage
	// Response mirrors AI SDK Node's response.messages for continuation.
	Response Response
}

// PrepareStepContext provides information about the current step for the PrepareStep callback.
type PrepareStepContext struct {
	StepNumber     int
	Steps          []PrepareStepInfo
	ToolsContext   ToolsContext
	RuntimeContext RuntimeContext
}

// PrepareStepInfo holds information about a completed step for PrepareStep evaluation.
type PrepareStepInfo struct {
	StepNumber   int
	HasToolCalls bool
	ToolNames    []string
	Text         string
	FinishReason FinishReason
}

// PrepareStepResult holds per-step overrides returned by PrepareStep.
type PrepareStepResult struct {
	Model           LanguageModel
	ToolChoice      *ToolChoice
	ActiveTools     []string
	Instructions    string
	ProviderOptions map[string]any
}

// PrepareStepFunc is called before each step to allow per-step configuration overrides.
type PrepareStepFunc func(ctx PrepareStepContext) *PrepareStepResult

// StepEndEvent is passed to the OnStepEnd callback after each step.
type StepEndEvent struct {
	StepNumber       int
	Text             string
	Reasoning        string
	ToolCalls        []ToolCallOutput
	ToolResults      []ToolResult
	FinishReason     FinishReason
	Usage            *Usage
	ProviderMetadata map[string]any
	Warnings         []Warning
	Response         Response
}

// EndEvent is passed to the OnEnd callback when the entire run completes.
type EndEvent struct {
	Text             string
	Reasoning        string
	Steps            []StepOutput
	Usage            Usage
	FinishReason     FinishReason
	ProviderMetadata map[string]any
	Response         Response
}

// ChunkEvent wraps a streaming engine event for the OnChunk callback.
type ChunkEvent struct {
	Type              string
	TextDelta         string
	ReasoningDelta    string
	ToolCallID        string
	ToolCallName      string
	ToolCallArgsDelta string
	StepNumber        int
	FinishReason      FinishReason
}

// GeneratedFile holds a file (typically an image) output from the model.
type GeneratedFile struct {
	// Data is the raw file bytes.
	Data []byte
	// MediaType is the MIME type of the file (e.g. "image/png").
	MediaType string
}
