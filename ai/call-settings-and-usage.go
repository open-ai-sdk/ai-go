package ai

import "github.com/open-ai-sdk/ai-go/aikit"
import "github.com/open-ai-sdk/ai-go/llm"

// Usage, token-detail, and FinishReason types are aliases of the shared aikit
// package so the ai and engine layers name one identical set (see ai/types.go).
type (
	CallSettings       = llm.CallSettings
	Usage              = aikit.Usage
	InputTokenDetails  = aikit.InputTokenDetails
	OutputTokenDetails = aikit.OutputTokenDetails
	FinishReason       = aikit.FinishReason
)

const (
	FinishReasonStop          = aikit.FinishReasonStop
	FinishReasonToolCalls     = aikit.FinishReasonToolCalls
	FinishReasonLength        = aikit.FinishReasonLength
	FinishReasonContentFilter = aikit.FinishReasonContentFilter
	FinishReasonError         = aikit.FinishReasonError
	FinishReasonUnknown       = aikit.FinishReasonUnknown
)
