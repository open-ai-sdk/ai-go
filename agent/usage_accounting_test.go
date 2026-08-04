package agent

import (
	"reflect"
	"testing"
)

func TestMergeUsageReplacesSameCallSnapshotsWithoutDroppingPartialFields(t *testing.T) {
	prior := &Usage{
		InputTokens:         10,
		InputTokenDetails:   InputTokenDetails{CacheReadTokens: 2},
		ToolUsePromptTokens: 3,
		Raw:                 map[string]any{"snapshot": map[string]any{"value": "prior"}},
	}
	incoming := &Usage{
		OutputTokens:       5,
		OutputTokenDetails: OutputTokenDetails{ReasoningTokens: 2},
		Raw:                map[string]any{"snapshot": map[string]any{"value": "incoming"}},
	}

	got := mergeUsage(prior, incoming)
	if got.InputTokens != 10 || got.InputTokenDetails.CacheReadTokens != 2 ||
		got.OutputTokens != 5 || got.OutputTokenDetails.ReasoningTokens != 2 ||
		got.ToolUsePromptTokens != 3 {
		t.Fatalf("mergeUsage() = %+v", got)
	}
	got.Raw["snapshot"].(map[string]any)["value"] = "changed"
	if incoming.Raw["snapshot"].(map[string]any)["value"] != "incoming" {
		t.Fatal("mergeUsage aliased the latest Raw snapshot")
	}
}

func TestEmitOnEndAddsEveryUsageCounter(t *testing.T) {
	first := Usage{
		InputTokens:        10,
		InputTokenDetails:  InputTokenDetails{NoCacheTokens: 7, CacheReadTokens: 2, CacheWriteTokens: 1},
		OutputTokens:       5,
		OutputTokenDetails: OutputTokenDetails{TextTokens: 3, ReasoningTokens: 2},
		TotalTokens:        19, ToolUsePromptTokens: 4,
		Raw: map[string]any{"step": float64(0)},
	}
	second := Usage{
		InputTokens:        6,
		InputTokenDetails:  InputTokenDetails{NoCacheTokens: 4, CacheReadTokens: 1, CacheWriteTokens: 1},
		OutputTokens:       4,
		OutputTokenDetails: OutputTokenDetails{TextTokens: 1, ReasoningTokens: 3},
		TotalTokens:        13, ToolUsePromptTokens: 3,
		Raw: map[string]any{"step": float64(1)},
	}
	var got endEvent
	emitOnEnd(&lifecycleCallbacks{OnEnd: func(event endEvent) { got = event }}, []StepResultInfo{
		{Usage: &first},
		{Usage: &second},
	}, streamResult{})

	want := first.Add(second)
	if !reflect.DeepEqual(got.Usage, want) {
		t.Fatalf("OnEnd Usage = %+v, want %+v", got.Usage, want)
	}
	got.Usage.Raw["step"] = float64(99)
	if second.Raw["step"] != float64(1) {
		t.Fatal("OnEnd Usage.Raw aliases the final step")
	}
}

func TestEmitOnEndUsesTerminalStreamMessageID(t *testing.T) {
	var got endEvent
	emitOnEnd(
		&lifecycleCallbacks{OnEnd: func(event endEvent) { got = event }},
		[]StepResultInfo{{MessageID: "older"}},
		streamResult{messageID: "terminal"},
	)
	if got.MessageID != "terminal" {
		t.Fatalf("MessageID = %q, want terminal", got.MessageID)
	}
}
