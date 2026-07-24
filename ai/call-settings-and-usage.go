package ai

import "github.com/open-ai-sdk/ai-go/aitypes"

// CallSettings controls model behavior per-request.
// All pointer fields are optional; nil means "use the model default".
type CallSettings struct {
	// Temperature controls randomness. Lower values make output more deterministic.
	Temperature *float32
	// MaxTokens limits the number of tokens in the completion (0 = model default).
	MaxTokens int
	// TopP enables nucleus sampling. Set either Temperature or TopP, not both.
	TopP *float32
	// TopK limits the next-token candidates to the top K options.
	// Not supported by all providers (e.g. OpenAI ignores it).
	TopK *int
	// Seed requests deterministic output. Support varies by provider.
	Seed *int
	// StopSequences causes the model to stop when any of these strings is generated.
	StopSequences []string
}

// Usage, token-detail, and FinishReason types are aliases of the shared aitypes
// package so the ai and engine layers name one identical set (see ai/types.go).
type (
	Usage              = aitypes.Usage
	InputTokenDetails  = aitypes.InputTokenDetails
	OutputTokenDetails = aitypes.OutputTokenDetails
	FinishReason       = aitypes.FinishReason
)

const (
	FinishReasonStop          = aitypes.FinishReasonStop
	FinishReasonToolCalls     = aitypes.FinishReasonToolCalls
	FinishReasonLength        = aitypes.FinishReasonLength
	FinishReasonContentFilter = aitypes.FinishReasonContentFilter
	FinishReasonError         = aitypes.FinishReasonError
	FinishReasonUnknown       = aitypes.FinishReasonUnknown
)
