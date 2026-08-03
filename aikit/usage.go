package aikit

import "github.com/open-ai-sdk/ai-go/internal/jsonclone"

// Usage holds token counts and the provider's raw usage payload.
type Usage struct {
	InputTokens         int
	InputTokenDetails   InputTokenDetails
	OutputTokens        int
	OutputTokenDetails  OutputTokenDetails
	TotalTokens         int
	ToolUsePromptTokens int
	// Raw contains the latest provider usage snapshot. Add clones JSON-like map,
	// slice, array, byte, and raw-message containers. Pointers, structs, channels,
	// and functions are treated as scalar values and therefore remain shared.
	Raw map[string]any
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

// HasValues reports whether at least one numeric usage counter was supplied.
// Raw provider metadata alone is not a token count.
func (u Usage) HasValues() bool {
	return u.InputTokens != 0 ||
		u.InputTokenDetails.NoCacheTokens != 0 ||
		u.InputTokenDetails.CacheReadTokens != 0 ||
		u.InputTokenDetails.CacheWriteTokens != 0 ||
		u.OutputTokens != 0 ||
		u.OutputTokenDetails.TextTokens != 0 ||
		u.OutputTokenDetails.ReasoningTokens != 0 ||
		u.TotalTokens != 0 ||
		u.ToolUsePromptTokens != 0
}

// Add returns the fieldwise total of two independently reported usages.
// TotalTokens is added exactly as supplied and is never inferred from details.
// Raw follows the latest non-nil snapshot policy and is independently cloned.
func (u Usage) Add(other Usage) Usage {
	raw := u.Raw
	if other.Raw != nil {
		raw = other.Raw
	}
	return Usage{
		InputTokens: u.InputTokens + other.InputTokens,
		InputTokenDetails: InputTokenDetails{
			NoCacheTokens:    u.InputTokenDetails.NoCacheTokens + other.InputTokenDetails.NoCacheTokens,
			CacheReadTokens:  u.InputTokenDetails.CacheReadTokens + other.InputTokenDetails.CacheReadTokens,
			CacheWriteTokens: u.InputTokenDetails.CacheWriteTokens + other.InputTokenDetails.CacheWriteTokens,
		},
		OutputTokens: u.OutputTokens + other.OutputTokens,
		OutputTokenDetails: OutputTokenDetails{
			TextTokens:      u.OutputTokenDetails.TextTokens + other.OutputTokenDetails.TextTokens,
			ReasoningTokens: u.OutputTokenDetails.ReasoningTokens + other.OutputTokenDetails.ReasoningTokens,
		},
		TotalTokens:         u.TotalTokens + other.TotalTokens,
		ToolUsePromptTokens: u.ToolUsePromptTokens + other.ToolUsePromptTokens,
		Raw:                 jsonclone.Map(raw),
	}
}

// Accumulate adds another independently reported usage into u.
func (u *Usage) Accumulate(other Usage) {
	*u = u.Add(other)
}
