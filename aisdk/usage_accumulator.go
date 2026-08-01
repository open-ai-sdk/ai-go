package aisdk

import "github.com/open-ai-sdk/ai-go/aikit"

// usageAccumulator treats each usage event as the latest cumulative snapshot
// for its step and adds only the change from the previous snapshot.
type usageAccumulator struct {
	total   UsageInfo
	current aikit.Usage
}

func (a *usageAccumulator) startStep() {
	a.current = aikit.Usage{}
}

func (a *usageAccumulator) apply(snapshot *aikit.Usage) {
	if snapshot == nil {
		return
	}
	a.total.InputTokens += snapshot.InputTokens - a.current.InputTokens
	a.total.InputTokenDetails.NoCacheTokens += snapshot.InputTokenDetails.NoCacheTokens - a.current.InputTokenDetails.NoCacheTokens
	a.total.InputTokenDetails.CacheReadTokens += snapshot.InputTokenDetails.CacheReadTokens - a.current.InputTokenDetails.CacheReadTokens
	a.total.InputTokenDetails.CacheWriteTokens += snapshot.InputTokenDetails.CacheWriteTokens - a.current.InputTokenDetails.CacheWriteTokens
	a.total.OutputTokens += snapshot.OutputTokens - a.current.OutputTokens
	a.total.OutputTokenDetails.TextTokens += snapshot.OutputTokenDetails.TextTokens - a.current.OutputTokenDetails.TextTokens
	a.total.OutputTokenDetails.ReasoningTokens += snapshot.OutputTokenDetails.ReasoningTokens - a.current.OutputTokenDetails.ReasoningTokens
	a.total.TotalTokens += snapshot.TotalTokens - a.current.TotalTokens
	a.current = *snapshot
}

func (a *usageAccumulator) snapshot() UsageInfo {
	return a.total
}
