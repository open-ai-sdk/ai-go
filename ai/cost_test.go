package ai_test

import (
	"math"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
)

const costEpsilon = 1e-12

// TestCalculateCost_CacheDecomposition verifies that a cache-inclusive
// InputTokens total is billed correctly: the non-cached remainder at the prompt
// rate, cache reads and writes at their own rates, with no double-billing of the
// cached portion at the full prompt rate.
func TestCalculateCost_CacheDecomposition(t *testing.T) {
	price := ai.ModelPrice{
		PromptPer1M:     3.00,
		CompletionPer1M: 15.00,
		CacheReadPer1M:  0.30,
		CacheWritePer1M: 3.75,
	}
	// InputTokens is the cache-inclusive total: 100 non-cached + 50 read + 20 write.
	usage := ai.Usage{
		InputTokens: 170,
		InputTokenDetails: ai.InputTokenDetails{
			NoCacheTokens:    100,
			CacheReadTokens:  50,
			CacheWriteTokens: 20,
		},
		OutputTokens: 200,
	}

	got := ai.CalculateCost("claude-4-sonnet", usage, price)

	want := 100*3.00/1e6 + 200*15.00/1e6 + 50*0.30/1e6 + 20*3.75/1e6
	if math.Abs(got.CostUSD-want) > costEpsilon {
		t.Errorf("CostUSD = %v, want %v", got.CostUSD, want)
	}
	if got.CacheReadTokens != 50 || got.CacheWriteTokens != 20 {
		t.Errorf("cache tokens = %d/%d, want 50/20", got.CacheReadTokens, got.CacheWriteTokens)
	}
	if got.PromptTokens != 170 {
		t.Errorf("PromptTokens = %d, want 170 (cache-inclusive total)", got.PromptTokens)
	}
}

// TestCalculateCost_NoDoubleBillingOfCache locks the phase mitigation: the
// cached portion of a cache-inclusive InputTokens total must not also be billed
// at the full prompt rate. The computed USD must equal the decomposed cost and
// be strictly less than naively billing the whole total at the prompt rate on
// top of the separate cache charges.
func TestCalculateCost_NoDoubleBillingOfCache(t *testing.T) {
	price := ai.ModelPrice{
		PromptPer1M:     3.00,
		CompletionPer1M: 15.00,
		CacheReadPer1M:  0.30,
		CacheWritePer1M: 3.75,
	}
	usage := ai.Usage{
		InputTokens:  170, // 100 non-cached + 50 read + 20 write
		OutputTokens: 200,
		InputTokenDetails: ai.InputTokenDetails{
			NoCacheTokens:    100,
			CacheReadTokens:  50,
			CacheWriteTokens: 20,
		},
	}

	got := ai.CalculateCost("claude-4-sonnet", usage, price)

	// Correct decomposition bills only the non-cached remainder at the prompt rate.
	decomposed := 100*3.00/1e6 + 200*15.00/1e6 + 50*0.30/1e6 + 20*3.75/1e6
	if math.Abs(got.CostUSD-decomposed) > costEpsilon {
		t.Errorf("CostUSD = %v, want decomposed %v", got.CostUSD, decomposed)
	}

	// The double-billing bug would charge the full 170 at the prompt rate.
	doubleBilled := 170*3.00/1e6 + 200*15.00/1e6 + 50*0.30/1e6 + 20*3.75/1e6
	if got.CostUSD >= doubleBilled {
		t.Errorf("CostUSD %v must be less than double-billed %v", got.CostUSD, doubleBilled)
	}
}

// TestCalculateCost_NoCacheSimple verifies the common no-cache case: the whole
// input bills at the prompt rate.
func TestCalculateCost_NoCacheSimple(t *testing.T) {
	price := ai.ModelPrice{PromptPer1M: 2.50, CompletionPer1M: 10.00}
	usage := ai.Usage{InputTokens: 1000, OutputTokens: 500}

	got := ai.CalculateCost("gpt-4o", usage, price)

	want := 1000*2.50/1e6 + 500*10.00/1e6
	if math.Abs(got.CostUSD-want) > costEpsilon {
		t.Errorf("CostUSD = %v, want %v", got.CostUSD, want)
	}
}
