package aisdk

// FinishReason indicates why the model stopped generating.
type FinishReason string

const (
	FinishReasonStop          FinishReason = "stop"
	FinishReasonToolCalls     FinishReason = "tool_calls"
	FinishReasonLength        FinishReason = "length"
	FinishReasonContentFilter FinishReason = "content_filter"
	FinishReasonError         FinishReason = "error"
	FinishReasonUnknown       FinishReason = "unknown"
)

// Usage holds token counts for a completion step.
type Usage struct {
	InputTokens        int
	InputTokenDetails  InputTokenDetails
	OutputTokens       int
	OutputTokenDetails OutputTokenDetails
	TotalTokens        int
	Raw                map[string]any
}

type InputTokenDetails struct {
	NoCacheTokens    int
	CacheReadTokens  int
	CacheWriteTokens int
}

type OutputTokenDetails struct {
	TextTokens      int
	ReasoningTokens int
}
