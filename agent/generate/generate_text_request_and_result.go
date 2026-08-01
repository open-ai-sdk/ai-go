package generate

import (
	"encoding/json"
	"log/slog"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

// GenerateTextRequest is the explicit input to GenerateText and StreamText.
// New code can prefer [NewRequest] for fluent construction.
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
	// StopWhen is a custom stop condition for the tool loop. Nil defaults to
	// IsStepCount(1) — a single step. Tool calls made in that step still
	// execute; only a follow-up model call is gated. Use IsStepCount(n),
	// Never(), or a custom StopCondition for a real multi-step loop.
	StopWhen StopCondition
	// Output optionally constrains the model's output to a JSON schema or mode.
	Output *OutputSchema
	// Settings controls per-request model parameters (temperature, maxTokens, etc.).
	Settings CallSettings
	// MaxSteps caps the number of tool-loop iterations. Zero means unbounded:
	// StopWhen (or the model naturally stopping — no tool calls) becomes the
	// only gate. There is no implicit default step count.
	MaxSteps int
	// ProviderOptions carries provider-specific options keyed by provider name.
	// Prefer typed options through RequestBuilder.With. A map value is reserved
	// for input decoded from JSON.
	ProviderOptions map[string]any
	// PrepareStep is called before each tool-loop step to allow per-step overrides.
	PrepareStep PrepareStepFunc
	// RepairToolCall attempts to repair invalid or unknown tool calls
	// before they are surfaced as invalid.
	RepairToolCall RepairToolCallFunc
	// ActiveTools filters the tool set to only these tool names. Nil means all tools.
	ActiveTools    []string
	ToolsContext   ToolsContext
	RuntimeContext RuntimeContext
	ToolApproval   map[string]ToolApprovalFunc
	// ToolApprovalKey authenticates approval requests resumed through message
	// history and signs completed results from synchronous responders. Every
	// approval-gated call requires at least 32 random bytes; keep the same key
	// across continuations. Derive separate keys for separate authenticated
	// audiences because the frozen v1 wire signature contains no tenant field.
	ToolApprovalKey []byte
	// ToolApprovalReplayGuard atomically reserves approved capabilities before
	// tools run and completes them afterward. It is required for approved
	// history resume. Production deployments should use a bounded, durable
	// lease/fencing implementation shared by every server instance.
	ToolApprovalReplayGuard ToolApprovalReplayGuard
	ToolApprovalResponder   ToolApprovalResponder
	// OnStepEnd is called after each step completes.
	OnStepEnd func(StepEndEvent)
	// OnEnd is called when the entire run completes.
	OnEnd func(EndEvent)
	// OnChunk is called for every engine event during streaming.
	OnChunk func(ChunkEvent)
	// OnError is called when an error occurs during the run.
	OnError func(error)
	// SmoothStream enables smooth text streaming with configurable chunking.
	// Applied by StreamText; GenerateText explicitly disables it so a
	// non-streaming call pays no per-chunk delay.
	SmoothStream *SmoothStream
	// Middlewares holds deferred model middlewares set via WithMiddleware or
	// WithRetry. Applied (and cleared) by StreamText, and earlier by
	// Runtime.buildRequest on the facade path — both honour them.
	Middlewares []LanguageModelMiddleware
	// ParallelToolExecution enables parallel execution of tool calls within a step.
	ParallelToolExecution bool
	// MaxParallelTools limits concurrent tool executions. Default: 5.
	MaxParallelTools int
	// Logger, when set via WithLogger, receives structured diagnostics
	// (recovered panics, dropped provider error bodies). Nil — the
	// default — produces no output at all; the SDK never writes to
	// slog.Default().
	Logger *slog.Logger
	// Tracer enables provider-neutral span instrumentation. Nil is a true no-op.
	Tracer agent.Tracer
	// TraceContent, when set via WithTraceContent(true), attaches prompt,
	// completion, and tool-argument content to trace spans. Default false:
	// spans carry only metadata (model ID, step number, tool name, usage,
	// finish reason), never content.
	TraceContent bool
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
	// Response contains messages for continuation.
	Response Response
}

// ToolCallOutput holds the details of a single tool call made by the model.
type ToolCallOutput = aikit.ToolCallInfo

// GenerateTextResult holds the full output of a GenerateText call.
type GenerateTextResult struct {
	Text      string
	Reasoning string
	Steps     []StepOutput
	// FinalStep contains the complete final-step output. It is zero when no step completed.
	FinalStep   StepOutput
	ToolResults []ToolResult
	// ToolApprovalRequests contains calls that suspended awaiting a stateless
	// approval decision. Their signatures must be echoed through
	// ToolApprovalResponsePart on resume.
	ToolApprovalRequests []ToolApprovalRequest
	Usage                Usage
	FinishReason         FinishReason
	RawFinishReason      string
	ProviderMetadata     map[string]any
	Warnings             []Warning
	Sources              []Source
	// Files holds file/image outputs from the model (aggregated across all steps).
	Files            []GeneratedFile
	StructuredOutput json.RawMessage
	// Response contains messages for continuation.
	Response Response
}

type (
	PrepareStepContext = llm.PrepareStepContext
	PrepareStepInfo    = llm.PrepareStepInfo
	PrepareStepResult  = llm.PrepareStepResult
	PrepareStepFunc    = llm.PrepareStepFunc
)

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
	ApprovalID        string
	ApprovalSignature string
	StepNumber        int
	FinishReason      FinishReason
	// Typed payloads previously dropped by the flattening converter. Each is set
	// only for the matching Type; consumers no longer lose Source/File/Usage/
	// ToolResult/metadata data on an OnChunk callback. Note: the slice and map
	// fields make ChunkEvent non-comparable with ==.
	Usage            *Usage
	Source           *Source
	ToolResult       *ToolResult
	FileData         []byte
	FileMediaType    string
	ProviderMetadata map[string]any
}

// GeneratedFile holds a file (typically an image) output from the model.
type GeneratedFile struct {
	// Data is the raw file bytes.
	Data []byte
	// MediaType is the MIME type of the file (e.g. "image/png").
	MediaType string
}
