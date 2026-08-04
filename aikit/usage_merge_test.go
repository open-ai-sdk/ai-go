package aikit

import (
	"reflect"
	"testing"
)

// The Anthropic pattern: input and cache counts arrive with the first usage
// event and the output count only with the last, so the later report must not
// zero the earlier one.
func TestUsageMergeKeepsEarlierCountsAgainstLaterZeros(t *testing.T) {
	prior := Usage{
		InputTokens:       120,
		InputTokenDetails: InputTokenDetails{CacheReadTokens: 40, CacheWriteTokens: 10},
	}
	merged := prior.Merge(Usage{OutputTokens: 35, TotalTokens: 155})

	if merged.InputTokens != 120 {
		t.Errorf("InputTokens = %d, want 120", merged.InputTokens)
	}
	if merged.InputTokenDetails.CacheReadTokens != 40 {
		t.Errorf("CacheReadTokens = %d, want 40", merged.InputTokenDetails.CacheReadTokens)
	}
	if merged.InputTokenDetails.CacheWriteTokens != 10 {
		t.Errorf("CacheWriteTokens = %d, want 10", merged.InputTokenDetails.CacheWriteTokens)
	}
	if merged.OutputTokens != 35 {
		t.Errorf("OutputTokens = %d, want 35", merged.OutputTokens)
	}
	if merged.TotalTokens != 155 {
		t.Errorf("TotalTokens = %d, want 155", merged.TotalTokens)
	}
}

func TestUsageMergeReplacesEveryNonZeroField(t *testing.T) {
	prior := Usage{
		InputTokens:         1,
		InputTokenDetails:   InputTokenDetails{NoCacheTokens: 2, CacheReadTokens: 3, CacheWriteTokens: 4},
		OutputTokens:        5,
		OutputTokenDetails:  OutputTokenDetails{TextTokens: 6, ReasoningTokens: 7},
		TotalTokens:         8,
		ToolUsePromptTokens: 9,
	}
	incoming := Usage{
		InputTokens:         10,
		InputTokenDetails:   InputTokenDetails{NoCacheTokens: 20, CacheReadTokens: 30, CacheWriteTokens: 40},
		OutputTokens:        50,
		OutputTokenDetails:  OutputTokenDetails{TextTokens: 60, ReasoningTokens: 70},
		TotalTokens:         80,
		ToolUsePromptTokens: 90,
	}
	merged := prior.Merge(incoming)

	if !reflect.DeepEqual(merged, incoming) {
		t.Errorf("Merge() = %+v, want every field replaced by %+v", merged, incoming)
	}
}

// Merge is the partial-report strategy: unlike Add it never sums.
func TestUsageMergeDoesNotAccumulate(t *testing.T) {
	merged := Usage{InputTokens: 100}.Merge(Usage{InputTokens: 5})
	if merged.InputTokens != 5 {
		t.Errorf("InputTokens = %d, want 5 (replace, not sum)", merged.InputTokens)
	}
}

func TestUsageMergeKeepsPriorRawWhenIncomingHasNone(t *testing.T) {
	prior := Usage{Raw: map[string]any{"provider": "kept"}}
	merged := prior.Merge(Usage{OutputTokens: 3})

	if got := merged.Raw["provider"]; got != "kept" {
		t.Errorf("Raw[provider] = %#v, want kept", got)
	}
}

func TestUsageMergeTakesLatestRawSnapshot(t *testing.T) {
	prior := Usage{Raw: map[string]any{"generation": "first"}}
	merged := prior.Merge(Usage{Raw: map[string]any{"generation": "second"}})

	if got := merged.Raw["generation"]; got != "second" {
		t.Errorf("Raw[generation] = %#v, want second", got)
	}
}

// Raw is cloned to the same depth as Add, so a merged usage never aliases a
// container the provider decoder still owns.
func TestUsageMergeClonesRawDeeply(t *testing.T) {
	nested := map[string]any{"cached": float64(2)}
	incoming := Usage{Raw: map[string]any{"details": nested}}
	merged := Usage{}.Merge(incoming)

	nested["cached"] = float64(99)

	details, ok := merged.Raw["details"].(map[string]any)
	if !ok {
		t.Fatalf("Raw[details] = %#v, want a map", merged.Raw["details"])
	}
	if details["cached"] != float64(2) {
		t.Errorf("Raw[details][cached] = %#v, want 2 — the merged usage aliases provider state", details["cached"])
	}
}

func TestUsageMergeLeavesReceiverUnchanged(t *testing.T) {
	prior := Usage{InputTokens: 7, Raw: map[string]any{"a": "b"}}
	_ = prior.Merge(Usage{InputTokens: 9, Raw: map[string]any{"c": "d"}})

	if prior.InputTokens != 7 {
		t.Errorf("receiver InputTokens = %d, want 7", prior.InputTokens)
	}
	if _, exists := prior.Raw["c"]; exists {
		t.Error("receiver Raw gained the incoming key")
	}
}
