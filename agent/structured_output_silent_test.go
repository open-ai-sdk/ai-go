package agent

import (
	"context"
	"testing"
)

// The finishing structured-output call is deliberately silent: its JSON is
// published only as StepEventStructuredOutput, never as text or reasoning
// deltas a UI would render. Sharing the usage-merge strategy with the other
// folds must not turn it into an ordinary stream consumer.
func TestStructuredOutputFinishingCallEmitsNoContentDeltas(t *testing.T) {
	exec := &mockExecutor{results: map[string]string{"get_time": `{"time":"12:00"}`}}
	model := &mockModel{calls: [][]StreamEvent{
		// Step 1 runs a tool, which is what lets StopWhen fire and reach the
		// separate finishing call.
		{toolCallEvt(0, "tc1", "get_time", `{"tz":"UTC"}`), finishEvt(FinishReasonToolCalls)},
		// The finishing call. Its text must not surface as deltas.
		{
			textEvt(`{"score":`),
			StreamEvent{Type: StreamEventReasoningDelta, TextDelta: "deciding"},
			textEvt(`42}`),
			StreamEvent{Type: StreamEventUsage, Usage: &Usage{InputTokens: 9}},
			StreamEvent{Type: StreamEventUsage, Usage: &Usage{OutputTokens: 3}},
			finishEvt(FinishReasonStop),
		},
	}}

	ch := driveStream(context.Background(), runConfig{
		Model:    model,
		Tools:    testToolSet(nil, exec),
		MaxSteps: 5,
		Request: Request{Output: &OutputSchema{
			Type:   "object",
			Schema: map[string]any{"type": "object"},
		}},
		StopWhen: func(int, *StepResult) bool { return true },
	})

	var events []StepEvent
	for event := range ch {
		events = append(events, event)
	}

	if model.idx != 2 {
		t.Fatalf("model calls = %d, want 2 — the finishing call was never reached", model.idx)
	}

	// Only the first call's step contributes deltas, and it emitted none.
	for _, event := range events {
		switch event.Type {
		case StepEventTextDelta:
			t.Errorf("finishing call leaked a text delta: %q", event.TextDelta)
		case StepEventReasoningDelta:
			t.Errorf("finishing call leaked a reasoning delta: %q", event.ReasoningDelta)
		}
	}

	structured, ok := findStructuredOutput(events)
	if !ok {
		t.Fatalf("events = %#v, want StepEventStructuredOutput", events)
	}
	if string(structured.StructuredOutput) != `{"score":42}` {
		t.Errorf("StructuredOutput = %s, want {\"score\":42}", structured.StructuredOutput)
	}
}

// Partial usage reports from the finishing call must merge rather than let the
// later report's zeros clobber the earlier counts.
func TestStructuredOutputFinishingCallMergesPartialUsage(t *testing.T) {
	prior := &Usage{InputTokens: 9}
	merged := mergeUsage(prior, &Usage{OutputTokens: 3})

	if merged.InputTokens != 9 || merged.OutputTokens != 3 {
		t.Fatalf("merged usage = %+v, want InputTokens 9 and OutputTokens 3", *merged)
	}
	if prior.OutputTokens != 0 {
		t.Error("mergeUsage mutated the prior usage in place")
	}
}
