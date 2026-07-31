package aikit

// Usage holds token counts and the provider's raw usage payload.
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
