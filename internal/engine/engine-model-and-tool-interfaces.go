package engine

import (
	"context"
	"log/slog"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/internal/tracing"
	"github.com/open-ai-sdk/ai-go/llm"
)

type ctxKey int

const toolCallIDCtxKey ctxKey = iota

// ToolCallIDFromContext returns the tool call ID injected by the engine.
func ToolCallIDFromContext(ctx context.Context) string {
	value, ok := ctx.Value(toolCallIDCtxKey).(string)
	if !ok {
		return ""
	}
	return value
}

// Shared model, message, request, loop, and tool-call contracts are aliases of
// aikit. The engine therefore consumes the same values providers and callers
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

	NoSuchToolError           = aikit.NoSuchToolError
	InvalidToolArgumentsError = aikit.InvalidToolArgumentsError
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

// LifecycleCallbacks holds optional observers for a run.
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

// EndEvent holds data passed to OnEnd when the run completes.
type EndEvent struct {
	Text             string
	Reasoning        string
	Steps            []StepResultInfo
	Usage            Usage
	FinishReason     FinishReason
	ProviderMetadata map[string]any
}

// RunParams configures a single engine run. It stays internal because tracing
// and logging are execution concerns rather than shared vocabulary.
type RunParams struct {
	Model                 Model
	Request               Request
	Tools                 *ToolSet
	StopWhen              StopCondition
	MaxSteps              int
	PrepareStep           PrepareStepFunc
	RepairToolCall        ToolCallRepairFunc
	ToolApproval          map[string]func(string, string) bool
	Approver              ApprovalResponder
	Callbacks             *LifecycleCallbacks
	ParallelToolExecution bool
	MaxParallelTools      int
	Logger                *slog.Logger
	Tracer                tracing.Tracer
	TraceContent          bool
}
