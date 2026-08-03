package agent

import (
	"log/slog"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/internal/tracing"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/tool"
)

// Shared model, message, request, loop, and tool-call contracts are aliases of
// aikit. The agent therefore consumes the same values providers and callers
// construct; no adapter or field-by-field conversion is needed.
type (
	Model                 = llm.Model
	Message               = aikit.Message
	ContentPart           = aikit.ContentPart
	ToolChoice            = aikit.ToolChoice
	Request               = llm.Request
	OutputSchema          = llm.OutputSchema
	CallSettings          = llm.CallSettings
	StopCondition         = aikit.StopCondition
	StepResult            = aikit.StepResult
	PrepareStepContext    = llm.PrepareStepContext
	StepResultInfo        = llm.PrepareStepInfo
	PrepareStepResult     = llm.PrepareStepResult
	PrepareStepFunc       = llm.PrepareStepFunc
	ToolCallInfo          = aikit.ToolCallInfo
	ToolCallRepairContext = aikit.RepairToolCallInput
	ToolCallRepairFunc    = aikit.RepairToolCallFunc
	StepEventType         = aikit.StepEventType
	StepEvent             = aikit.StepEvent
	FinishReason          = aikit.FinishReason
	Usage                 = aikit.Usage
	InputTokenDetails     = aikit.InputTokenDetails
	OutputTokenDetails    = aikit.OutputTokenDetails
	ToolResultContent     = aikit.ToolResultContent
	ToolResult            = aikit.ToolResult
	StreamEventType       = aikit.StreamEventType
	Source                = aikit.Source
	Warning               = aikit.Warning
	StreamEvent           = aikit.StreamEvent

	NoSuchToolError = tool.NoSuchToolError
	ToolInputError  = tool.InputError
	// Attr, Span, and Tracer form the optional provider-neutral tracing seam.
	// The core defaults to a no-op tracer and imports no telemetry SDK.
	Attr   = tracing.Attr
	Span   = tracing.Span
	Tracer = tracing.Tracer
)

const (
	StepEventTextDelta           = aikit.StepEventTextDelta
	StepEventReasoningDelta      = aikit.StepEventReasoningDelta
	StepEventToolCallStart       = aikit.StepEventToolCallStart
	StepEventToolCallDelta       = aikit.StepEventToolCallDelta
	StepEventToolCallReady       = aikit.StepEventToolCallReady
	StepEventToolResult          = aikit.StepEventToolResult
	StepEventToolApprovalRequest = aikit.StepEventToolApprovalRequest
	StepEventToolOutputDenied    = aikit.StepEventToolOutputDenied
	StepEventUsage               = aikit.StepEventUsage
	StepEventStepStart           = aikit.StepEventStepStart
	StepEventStepEnd             = aikit.StepEventStepEnd
	StepEventToolCallInvalid     = aikit.StepEventToolCallInvalid
	StepEventStructuredOutput    = aikit.StepEventStructuredOutput
	StepEventDone                = aikit.StepEventDone
	StepEventError               = aikit.StepEventError
	StepEventSource              = aikit.StepEventSource
	StepEventFileDelta           = aikit.StepEventFileDelta

	FinishReasonStop          = aikit.FinishReasonStop
	FinishReasonToolCalls     = aikit.FinishReasonToolCalls
	FinishReasonLength        = aikit.FinishReasonLength
	FinishReasonContentFilter = aikit.FinishReasonContentFilter
	FinishReasonError         = aikit.FinishReasonError
	FinishReasonUnknown       = aikit.FinishReasonUnknown

	ToolResultContentTypeText = aikit.ToolResultContentTypeText
	ToolResultContentTypeFile = aikit.ToolResultContentTypeFile

	StreamEventTextDelta      = aikit.StreamEventTextDelta
	StreamEventReasoningDelta = aikit.StreamEventReasoningDelta
	StreamEventToolCallDelta  = aikit.StreamEventToolCallDelta
	StreamEventUsage          = aikit.StreamEventUsage
	StreamEventFinish         = aikit.StreamEventFinish
	StreamEventError          = aikit.StreamEventError
	StreamEventSource         = aikit.StreamEventSource
	StreamEventFileDelta      = aikit.StreamEventFileDelta
)

// lifecycleCallbacks is the internal callback bridge retained by the driver.
type lifecycleCallbacks struct {
	OnStepEnd func(event stepEndEvent)
	OnEnd     func(event endEvent)
	OnChunk   func(event StepEvent)
	OnError   func(err error)
}

// stepEndEvent holds data passed to OnStepEnd after each step.
type stepEndEvent struct {
	StepNumber       int
	MessageID        string
	Text             string
	Reasoning        string
	ToolCalls        []ToolCallInfo
	ToolResults      []ToolResult
	FinishReason     FinishReason
	Usage            *Usage
	ProviderMetadata map[string]any
	Warnings         []Warning
}

// endEvent holds data passed to OnEnd when the run completes.
type endEvent struct {
	MessageID        string
	Text             string
	Reasoning        string
	Steps            []StepResultInfo
	Usage            Usage
	FinishReason     FinishReason
	ProviderMetadata map[string]any
}

// runConfig is the private driver snapshot produced by Runner.
type runConfig struct {
	Model    Model
	Request  Request
	Tools    *ToolSet
	StopWhen StopCondition
	MaxSteps int
	// ErrorOnMaxTurns makes exhaustion a typed terminal error instead of a
	// successful Done with pending tool calls. It is the canonical Runner
	// behavior; the field exists only while legacy callers are cut over.
	ErrorOnMaxTurns bool
	PrepareStep     PrepareStepFunc
	RepairToolCall  ToolCallRepairFunc
	ToolApproval    map[string]func(string, string) bool
	// ApprovalKey authenticates stateless approval requests and responses.
	// It must contain at least 32 bytes when an approval can suspend.
	ApprovalKey           []byte
	ApprovalReplayGuard   ApprovalReplayGuard
	Approver              ApprovalResponder
	Callbacks             *lifecycleCallbacks
	ParallelToolExecution bool
	MaxParallelTools      int
	Logger                *slog.Logger
	TraceContent          bool
	// Hooks and HookContext are snapshots owned by the canonical Runner.
	// Legacy low-level callers leave both empty.
	Hooks       []Hook
	HookContext HookContext
	// Tracer enables span instrumentation for this run. Nil is a true no-op
	// default; use an optional adapter or implement the small interface.
	Tracer Tracer
}
