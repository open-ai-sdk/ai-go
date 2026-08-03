package aikit

import "testing"

func TestUsageHasValues(t *testing.T) {
	tests := []struct {
		name  string
		usage Usage
		want  bool
	}{
		{name: "zero"},
		{name: "raw only", usage: Usage{Raw: map[string]any{"tokens": 1}}},
		{name: "input", usage: Usage{InputTokens: 1}, want: true},
		{name: "no cache", usage: Usage{InputTokenDetails: InputTokenDetails{NoCacheTokens: 1}}, want: true},
		{name: "cache read", usage: Usage{InputTokenDetails: InputTokenDetails{CacheReadTokens: 1}}, want: true},
		{name: "cache write", usage: Usage{InputTokenDetails: InputTokenDetails{CacheWriteTokens: 1}}, want: true},
		{name: "output", usage: Usage{OutputTokens: 1}, want: true},
		{name: "text", usage: Usage{OutputTokenDetails: OutputTokenDetails{TextTokens: 1}}, want: true},
		{name: "reasoning", usage: Usage{OutputTokenDetails: OutputTokenDetails{ReasoningTokens: 1}}, want: true},
		{name: "total", usage: Usage{TotalTokens: 1}, want: true},
		{name: "tool use prompt", usage: Usage{ToolUsePromptTokens: 1}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.usage.HasValues(); got != test.want {
				t.Fatalf("HasValues() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestUsageAddIsFieldwiseAndIndependent(t *testing.T) {
	leftRaw := map[string]any{"nested": map[string]any{"value": "left"}}
	rightRaw := map[string]any{"nested": map[string]any{"value": "right"}}
	left := Usage{
		InputTokens: 1, InputTokenDetails: InputTokenDetails{NoCacheTokens: 2, CacheReadTokens: 3, CacheWriteTokens: 4},
		OutputTokens: 5, OutputTokenDetails: OutputTokenDetails{TextTokens: 6, ReasoningTokens: 7},
		TotalTokens: 8, ToolUsePromptTokens: 9, Raw: leftRaw,
	}
	right := Usage{
		InputTokens: 10,
		InputTokenDetails: InputTokenDetails{
			NoCacheTokens: 20, CacheReadTokens: 30, CacheWriteTokens: 40,
		},
		OutputTokens: 50, OutputTokenDetails: OutputTokenDetails{TextTokens: 60, ReasoningTokens: 70},
		TotalTokens: 80, ToolUsePromptTokens: 90, Raw: rightRaw,
	}

	got := left.Add(right)
	want := Usage{
		InputTokens: 11,
		InputTokenDetails: InputTokenDetails{
			NoCacheTokens: 22, CacheReadTokens: 33, CacheWriteTokens: 44,
		},
		OutputTokens: 55, OutputTokenDetails: OutputTokenDetails{TextTokens: 66, ReasoningTokens: 77},
		TotalTokens: 88, ToolUsePromptTokens: 99,
	}
	if got.InputTokens != want.InputTokens || got.InputTokenDetails != want.InputTokenDetails ||
		got.OutputTokens != want.OutputTokens || got.OutputTokenDetails != want.OutputTokenDetails ||
		got.TotalTokens != want.TotalTokens || got.ToolUsePromptTokens != want.ToolUsePromptTokens {
		t.Fatalf("Add() = %+v, want numeric fields %+v", got, want)
	}
	got.Raw["nested"].(map[string]any)["value"] = "changed"
	if rightRaw["nested"].(map[string]any)["value"] != "right" {
		t.Fatal("Add aliased the latest Raw map")
	}
	if left.Raw["nested"].(map[string]any)["value"] != "left" {
		t.Fatal("Add mutated the left operand")
	}
}

func TestUsageAccumulateMatchesAddAndClonesRaw(t *testing.T) {
	base := Usage{InputTokens: 2, Raw: map[string]any{"nested": map[string]any{"value": "base"}}}
	other := Usage{OutputTokens: 3, Raw: map[string]any{"nested": map[string]any{"value": "other"}}}
	want := base.Add(other)
	base.Accumulate(other)

	if base.InputTokens != want.InputTokens || base.OutputTokens != want.OutputTokens {
		t.Fatalf("Accumulate() = %+v, want %+v", base, want)
	}
	base.Raw["nested"].(map[string]any)["value"] = "changed"
	if other.Raw["nested"].(map[string]any)["value"] != "other" {
		t.Fatal("Accumulate aliased the incoming Raw map")
	}
}
