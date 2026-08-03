package ai

import (
	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/agent/generate"
	"github.com/open-ai-sdk/ai-go/llm"
)

// The ai package is an ergonomic facade. Concrete API contracts are owned by
// lower packages and re-exported here as aliases for source compatibility.
type (
	Agent                   = generate.Agent
	AgentOption             = generate.AgentOption
	ApprovalDecision        = generate.ApprovalDecision
	CallSettings            = generate.CallSettings
	Completion              = generate.Completion
	ChunkDetector           = generate.ChunkDetector
	ChunkEvent              = generate.ChunkEvent
	ContentPart             = generate.ContentPart
	ContentPartType         = generate.ContentPartType
	CostTracker             = generate.CostTracker
	EmbeddingModel          = generate.EmbeddingModel
	EndEvent                = generate.EndEvent
	FallbackModel           = generate.FallbackModel
	FinishReason            = generate.FinishReason
	GenerateImageRequest    = generate.GenerateImageRequest
	GenerateImageResult     = generate.GenerateImageResult
	GenerateObjectRequest   = generate.GenerateObjectRequest
	GenerateTextRequest     = generate.GenerateTextRequest
	GenerateTextResult      = generate.GenerateTextResult
	GeneratedFile           = generate.GeneratedFile
	GeneratedImage          = generate.GeneratedImage
	ImageInput              = generate.ImageInput
	ImageModel              = generate.ImageModel
	InputTokenDetails       = generate.InputTokenDetails
	LanguageModel           = generate.LanguageModel
	LanguageModelMiddleware = generate.LanguageModelMiddleware
	LanguageModelRequest    = generate.LanguageModelRequest
	Message                 = generate.Message
	ModelPrice              = generate.ModelPrice
	NoSuchToolError         = generate.NoSuchToolError
	Option                  = generate.Option
	OutputSchema            = generate.OutputSchema
	OutputTokenDetails      = generate.OutputTokenDetails
	PrepareStepContext      = generate.PrepareStepContext
	PrepareStepFunc         = generate.PrepareStepFunc
	PrepareStepInfo         = generate.PrepareStepInfo
	PrepareStepResult       = generate.PrepareStepResult
	PruneMode               = generate.PruneMode
	PruneOptions            = generate.PruneOptions
	Registry                = generate.Registry
	RepairToolCallFunc      = generate.RepairToolCallFunc
	RepairToolCallInput     = generate.RepairToolCallInput
	RequestBuilder          = generate.RequestBuilder
	Response                = generate.Response
	RetryConfig             = generate.RetryConfig
	Role                    = generate.Role
	Runtime                 = generate.Runtime
	RuntimeContext          = generate.RuntimeContext
	RuntimeOption           = generate.RuntimeOption
	SmoothStream            = generate.SmoothStream
	SmoothStreamOption      = generate.SmoothStreamOption
	Source                  = generate.Source
	StepCost                = generate.StepCost
	StepEndEvent            = generate.StepEndEvent
	StepEvent               = generate.StepEvent
	StepEventType           = generate.StepEventType
	StepOutput              = generate.StepOutput
	StepResult              = generate.StepResult
	StopCondition           = generate.StopCondition
	StreamEvent             = generate.StreamEvent
	StreamEventType         = generate.StreamEventType
	StreamResult            = generate.StreamResult
	StreamingToolExecutor   = generate.StreamingToolExecutor
	ToolApprovalFunc        = generate.ToolApprovalFunc
	ToolApprovalGrant       = generate.ToolApprovalGrant
	ToolApprovalReplayGuard = generate.ToolApprovalReplayGuard
	ToolApprovalRequest     = generate.ToolApprovalRequest
	ToolApprovalResponder   = generate.ToolApprovalResponder
	ToolApprovalResponse    = generate.ToolApprovalResponse
	ToolCallOutput          = generate.ToolCallOutput
	ToolChoice              = generate.ToolChoice
	ToolDefinition          = generate.ToolDefinition
	ToolExecutor            = generate.ToolExecutor
	ToolInputError          = generate.ToolInputError
	ToolLoopAgent           = generate.ToolLoopAgent
	ToolResult              = generate.ToolResult
	ToolResultContent       = generate.ToolResultContent
	ToolResultStream        = generate.ToolResultStream
	ToolSet                 = generate.ToolSet
	ToolsContext            = generate.ToolsContext
	Tracer                  = agent.Tracer
	Usage                   = generate.Usage
	Warning                 = generate.Warning
)

// Direct completion contracts are owned by llm because they use the model
// boundary without entering the agent runtime.
type (
	CompletionRequest        = llm.CompletionRequest
	CompletionRequestBuilder = llm.CompletionRequestBuilder
	CompletionResponse       = llm.CompletionResponse
	CompletionFile           = llm.GeneratedFile
)

// AgentCompletionRequestBuilder belongs to the agent runtime and runs its
// full tool loop.
type AgentCompletionRequestBuilder = generate.AgentCompletionRequestBuilder

type ObjectResult[T any] = generate.ObjectResult[T]

const (
	ApprovalRequired = generate.ApprovalRequired

	RoleSystem    = generate.RoleSystem
	RoleUser      = generate.RoleUser
	RoleAssistant = generate.RoleAssistant
	RoleTool      = generate.RoleTool

	ContentPartTypeText                 = generate.ContentPartTypeText
	ContentPartTypeFile                 = generate.ContentPartTypeFile
	ContentPartTypeToolCall             = generate.ContentPartTypeToolCall
	ContentPartTypeToolResult           = generate.ContentPartTypeToolResult
	ContentPartTypeToolApprovalResponse = generate.ContentPartTypeToolApprovalResponse
	ContentPartTypeReasoning            = generate.ContentPartTypeReasoning

	FinishReasonStop          = generate.FinishReasonStop
	FinishReasonToolCalls     = generate.FinishReasonToolCalls
	FinishReasonLength        = generate.FinishReasonLength
	FinishReasonContentFilter = generate.FinishReasonContentFilter
	FinishReasonError         = generate.FinishReasonError
	FinishReasonUnknown       = generate.FinishReasonUnknown

	PruneModeNone            = generate.PruneModeNone
	PruneModeAll             = generate.PruneModeAll
	PruneModeBeforeLastMsg   = generate.PruneModeBeforeLastMsg
	PruneModeBeforeLastNMsgs = generate.PruneModeBeforeLastNMsgs
	PruneModeRemove          = generate.PruneModeRemove
	PruneModeKeep            = generate.PruneModeKeep

	StreamEventTextDelta      = generate.StreamEventTextDelta
	StreamEventReasoningDelta = generate.StreamEventReasoningDelta
	StreamEventToolCallDelta  = generate.StreamEventToolCallDelta
	StreamEventUsage          = generate.StreamEventUsage
	StreamEventFinish         = generate.StreamEventFinish
	StreamEventError          = generate.StreamEventError
	StreamEventSource         = generate.StreamEventSource
	StreamEventFileDelta      = generate.StreamEventFileDelta

	StepEventTextDelta           = generate.StepEventTextDelta
	StepEventReasoningDelta      = generate.StepEventReasoningDelta
	StepEventToolCallStart       = generate.StepEventToolCallStart
	StepEventToolCallDelta       = generate.StepEventToolCallDelta
	StepEventToolCallReady       = generate.StepEventToolCallReady
	StepEventToolResult          = generate.StepEventToolResult
	StepEventToolApprovalRequest = generate.StepEventToolApprovalRequest
	StepEventToolOutputDenied    = generate.StepEventToolOutputDenied
	StepEventUsage               = generate.StepEventUsage
	StepEventStepStart           = generate.StepEventStepStart
	StepEventStepEnd             = generate.StepEventStepEnd
	StepEventToolCallInvalid     = generate.StepEventToolCallInvalid
	StepEventStructuredOutput    = generate.StepEventStructuredOutput
	StepEventDone                = generate.StepEventDone
	StepEventError               = generate.StepEventError
	StepEventSource              = generate.StepEventSource
	StepEventFileDelta           = generate.StepEventFileDelta

	ToolResultContentTypeText = generate.ToolResultContentTypeText
	ToolResultContentTypeFile = generate.ToolResultContentTypeFile
)

var (
	ErrStreamConsumed  = generate.ErrStreamConsumed
	ToolChoiceAuto     = generate.ToolChoiceAuto
	ToolChoiceNone     = generate.ToolChoiceNone
	ToolChoiceRequired = generate.ToolChoiceRequired
)
