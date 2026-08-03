package aikit

import "testing"

type (
	namedUsageMap   map[string]any
	namedUsageSlice []any
)

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

func TestUsageAddPreservesRawAliasesWithinClone(t *testing.T) {
	shared := map[string]any{"value": "original"}
	bytes := []byte("original")
	usage := Usage{Raw: map[string]any{
		"null":         nil,
		"first":        shared,
		"second":       shared,
		"bytes-first":  bytes,
		"bytes-second": bytes,
	}}

	cloned := (Usage{}).Add(usage).Raw
	if cloned["null"] != nil {
		t.Fatalf("null value = %#v, want nil", cloned["null"])
	}
	cloned["first"].(map[string]any)["value"] = "changed"
	cloned["bytes-first"].([]byte)[0] = 'X'
	if got := cloned["second"].(map[string]any)["value"]; got != "changed" {
		t.Fatalf("map alias value = %q, want changed", got)
	}
	if got := cloned["bytes-second"].([]byte)[0]; got != 'X' {
		t.Fatalf("byte alias value = %q, want X", got)
	}
	if shared["value"] != "original" || bytes[0] != 'o' {
		t.Fatal("clone mutation leaked to raw usage source")
	}
}

func TestUsageAddClonesNamedFallbackContainersAndCycles(t *testing.T) {
	named := namedUsageMap{"value": "original"}
	named["self"] = named
	var nilSlice namedUsageSlice
	usage := Usage{Raw: map[string]any{
		"named":     named,
		"nil-slice": nilSlice,
	}}

	cloned := (Usage{}).Add(usage).Raw
	clonedNamed := cloned["named"].(namedUsageMap)
	clonedNamed["value"] = "changed"
	self := clonedNamed["self"].(namedUsageMap)
	if self["value"] != "changed" {
		t.Fatalf("named cycle value = %q, want changed", self["value"])
	}
	if named["value"] != "original" {
		t.Fatal("named fallback clone mutation leaked to source")
	}
	if cloned["nil-slice"].(namedUsageSlice) != nil {
		t.Fatal("named typed nil slice became non-nil")
	}
}
