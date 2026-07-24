package engine

import (
	"context"
	"log/slog"

	"github.com/open-ai-sdk/ai-go/aitypes"
)

type ctxKey int

const toolCallIDCtxKey ctxKey = iota

// ToolCallIDFromContext returns the tool call ID injected by the engine
// when executing a tool. Returns empty string if not in a tool execution.
func ToolCallIDFromContext(ctx context.Context) string {
	v, ok := ctx.Value(toolCallIDCtxKey).(string)
	if !ok {
		return ""
	}
	return v
}

// Model is the engine-internal interface a language model provider must satisfy.
// It mirrors ai.LanguageModel but uses engine-local types to avoid import cycles.
type Model interface {
	ModelID() string
	Stream(ctx context.Context, req Request) (<-chan StreamEvent, error)
}

// Message is a conversation turn (engine-internal).
type Message struct {
	Role    string
	Content []ContentPart
}

// ContentPart is a single part of a message.
// Type is one of: "text", "file", "tool_call", "tool_result", "reasoning".
// Images have no dedicated type — they are file parts carrying an image MediaType.
type ContentPart struct {
	Type string

	// text / reasoning
	Text string

	// file
	FileURL   string
	MediaType string

	// Shared multimodal fields (file parts).
	Data     []byte // Inline binary content.
	FileID   string // Provider-specific file identifier.
	Filename string // Original filename (file parts).

	// tool_call
	ToolCallID       string
	ToolCallName     string
	ToolCallArgs     string // JSON string
	ThoughtSignature string // Gemini thought signature for multi-turn

	// tool_result
	ToolResultID     string
	ToolResultName   string
	ToolResultOutput string
}

// ToolChoice controls which tool the model must call.
type ToolChoice struct {
	// Type is one of "auto", "none", "required", or "tool".
	Type string
	// ToolName is set when Type == "tool".
	ToolName string
}

// Request is the engine-internal model request.
type Request struct {
	Instructions    string
	Messages        []Message
	Tools           []ToolDefinition
	ToolChoice      *ToolChoice
	Output          *OutputSchema
	Settings        CallSettings
	ProviderOptions map[string]any
	ToolsContext    map[string]any
	RuntimeContext  map[string]any
}

// OutputSchema describes the desired JSON structure for a structured output call.
type OutputSchema struct {
	Type   string
	Schema map[string]any
}

// CallSettings controls model behavior per-request.
type CallSettings struct {
	Temperature   *float32
	MaxTokens     int
	TopP          *float32
	TopK          *int
	Seed          *int
	StopSequences []string
}

// StopCondition determines when the tool loop should stop.
type StopCondition func(step int, result *StepResult) bool

// StepResult holds information about a completed step for stop condition evaluation.
type StepResult struct {
	HasToolCalls bool
	ToolNames    []string
	Text         string
}

// PrepareStepContext provides information about the current step for the PrepareStep callback.
type PrepareStepContext struct {
	StepNumber     int
	Steps          []StepResultInfo
	ToolsContext   map[string]any
	RuntimeContext map[string]any
}

// StepResultInfo holds information about a completed step for PrepareStep evaluation.
type StepResultInfo struct {
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

// PrepareStepResult holds per-step overrides returned by PrepareStep.
// Nil fields mean "no override" — use the base request value.
type PrepareStepResult struct {
	Model           Model
	ToolChoice      *ToolChoice
	ActiveTools     []string
	Instructions    string
	ProviderOptions map[string]any
}

// PrepareStepFunc is called before each step to allow per-step configuration overrides.
type PrepareStepFunc func(ctx PrepareStepContext) *PrepareStepResult

// LifecycleCallbacks holds optional callbacks for observability during a run.
type LifecycleCallbacks struct {
	OnStepEnd func(event StepEndEvent)
	OnEnd     func(event EndEvent)
	OnChunk   func(event StepEvent)
	OnError   func(err error)
}

// StepEndEvent holds data passed to OnStepEnd after each step.
type StepEndEvent struct {
	StepNumber       int
	Text             string
	Reasoning        string
	ToolCalls        []ToolCallInfo
	ToolResults      []ToolResult
	FinishReason     FinishReason
	Usage            *Usage
	ProviderMetadata map[string]any
	Warnings         []Warning
}

// EndEvent holds data passed to OnEnd when the entire run completes.
type EndEvent struct {
	Text             string
	Reasoning        string
	Steps            []StepResultInfo
	Usage            Usage
	FinishReason     FinishReason
	ProviderMetadata map[string]any
}

// ToolCallInfo describes a tool call for lifecycle callbacks.
type ToolCallInfo struct {
	ID   string
	Name string
	Args string
	// ArgsSet indicates whether Args should overwrite the original args when a
	// repair callback returns a replacement ToolCallInfo. Callers that only
	// change the tool name should leave this false to preserve the original args.
	ArgsSet          bool
	ThoughtSignature string
}

// ToolCallRepairContext describes a tool call that failed validation.
type ToolCallRepairContext struct {
	Instructions string
	Messages     []Message
	ToolCall     ToolCallInfo
	Tools        *ToolSet
	Error        error
}

// ToolCallRepairFunc attempts to repair an invalid tool call before it is surfaced.
type ToolCallRepairFunc func(ctx context.Context, input ToolCallRepairContext) (*ToolCallInfo, error)

// Tool error types are aliases of the shared aitypes definitions, so a value
// raised inside the engine loop is the same type the caller matches with
// errors.As/Is (see aitypes/tool-errors.go).
type (
	NoSuchToolError           = aitypes.NoSuchToolError
	InvalidToolArgumentsError = aitypes.InvalidToolArgumentsError
)

// RunParams configures a single engine run.
type RunParams struct {
	Model          Model
	Request        Request
	Tools          *ToolSet
	StopWhen       StopCondition
	MaxSteps       int
	PrepareStep    PrepareStepFunc
	RepairToolCall ToolCallRepairFunc
	ToolApproval   map[string]func(string, string) bool
	Approver       ApprovalResponder
	Callbacks      *LifecycleCallbacks
	// ParallelToolExecution enables parallel tool execution.
	ParallelToolExecution bool
	// MaxParallelTools limits concurrent tool executions. Default: 5.
	MaxParallelTools int
	// Logger, when set, receives structured logs (e.g. recovered panics). Nil is
	// a no-op.
	Logger *slog.Logger
}
