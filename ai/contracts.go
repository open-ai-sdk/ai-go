package ai

import (
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

// Package ai is a small convenience facade over the canonical leaf packages.
// Agent construction and execution live exclusively in package agent.
type (
	CallSettings         = llm.CallSettings
	ContentPart          = aikit.ContentPart
	ContentPartType      = aikit.ContentPartType
	EmbeddingModel       = llm.EmbeddingModel
	FinishReason         = aikit.FinishReason
	GenerateImageRequest = llm.GenerateImageRequest
	GenerateImageResult  = llm.GenerateImageResult
	GeneratedImage       = llm.GeneratedImage
	ImageInput           = llm.ImageInput
	ImageModel           = llm.ImageModel
	InputTokenDetails    = aikit.InputTokenDetails
	LanguageModel        = llm.Model
	LanguageModelRequest = llm.Request
	Message              = aikit.Message
	OutputSchema         = llm.OutputSchema
	OutputTokenDetails   = aikit.OutputTokenDetails
	Role                 = aikit.Role
	RuntimeContext       = aikit.RuntimeContext
	Source               = aikit.Source
	StreamEvent          = aikit.StreamEvent
	StreamEventType      = aikit.StreamEventType
	ToolChoice           = aikit.ToolChoice
	ToolDefinition       = aikit.ToolDefinition
	ToolResult           = aikit.ToolResult
	ToolResultContent    = aikit.ToolResultContent
	ToolsContext         = aikit.ToolsContext
	Usage                = aikit.Usage
	Warning              = aikit.Warning
)

type (
	CompletionRequest        = llm.CompletionRequest
	CompletionRequestBuilder = llm.CompletionRequestBuilder
	CompletionModel          = llm.CompletionModel
	CompletionResponse       = llm.CompletionResponse
	CompletionFile           = llm.GeneratedFile
)

const (
	RoleSystem    = aikit.RoleSystem
	RoleUser      = aikit.RoleUser
	RoleAssistant = aikit.RoleAssistant
	RoleTool      = aikit.RoleTool

	ContentPartTypeText                 = aikit.ContentPartTypeText
	ContentPartTypeFile                 = aikit.ContentPartTypeFile
	ContentPartTypeImage                = aikit.ContentPartTypeImage
	ContentPartTypeAudio                = aikit.ContentPartTypeAudio
	ContentPartTypeDocument             = aikit.ContentPartTypeDocument
	ContentPartTypeVideo                = aikit.ContentPartTypeVideo
	ContentPartTypeToolCall             = aikit.ContentPartTypeToolCall
	ContentPartTypeToolResult           = aikit.ContentPartTypeToolResult
	ContentPartTypeToolApprovalResponse = aikit.ContentPartTypeToolApprovalResponse
	ContentPartTypeReasoning            = aikit.ContentPartTypeReasoning

	FinishReasonStop          = aikit.FinishReasonStop
	FinishReasonToolCalls     = aikit.FinishReasonToolCalls
	FinishReasonLength        = aikit.FinishReasonLength
	FinishReasonContentFilter = aikit.FinishReasonContentFilter
	FinishReasonError         = aikit.FinishReasonError
	FinishReasonUnknown       = aikit.FinishReasonUnknown

	StreamEventTextDelta      = aikit.StreamEventTextDelta
	StreamEventReasoningDelta = aikit.StreamEventReasoningDelta
	StreamEventToolCallDelta  = aikit.StreamEventToolCallDelta
	StreamEventUsage          = aikit.StreamEventUsage
	StreamEventFinish         = aikit.StreamEventFinish
	StreamEventError          = aikit.StreamEventError
	StreamEventSource         = aikit.StreamEventSource
	StreamEventFileDelta      = aikit.StreamEventFileDelta

	ToolResultContentTypeText  = aikit.ToolResultContentTypeText
	ToolResultContentTypeFile  = aikit.ToolResultContentTypeFile
	ToolResultContentTypeJSON  = aikit.ToolResultContentTypeJSON
	ToolResultContentTypeImage = aikit.ToolResultContentTypeImage
)

var (
	ErrInvalidMessage   = aikit.ErrInvalidMessage
	ErrWrongMessageRole = aikit.ErrWrongMessageRole
	ToolChoiceAuto      = aikit.ToolChoice{Type: "auto"}
	ToolChoiceNone      = aikit.ToolChoice{Type: "none"}
	ToolChoiceRequired  = aikit.ToolChoice{Type: "required"}
)
