package ainode

import (
	"bytes"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// TestWriterMerge_BasicText verifies model stream events are written to the Writer.
func TestWriterMerge_BasicText(t *testing.T) {
	events := makeEventStream(
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "hi"},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)

	var buf bytes.Buffer
	wr := NewWriter(&buf)
	text := wr.Merge(events)

	output := buf.String()
	if text != "hi" {
		t.Errorf("expected text=%q, got %q", "hi", text)
	}
	assertContains(t, output, `"type":"text-delta"`)
	assertContains(t, output, `"delta":"hi"`)
	// Merge does NOT emit start or finish; caller manages lifecycle.
	assertNotContains(t, output, `"type":"finish"`)
}

// TestWriterMerge_CustomDataInterleaving verifies the full custom data + model
// stream interleaving pattern works correctly.
func TestWriterMerge_CustomDataInterleaving(t *testing.T) {
	events := makeEventStream(
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "answer"},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)

	var buf bytes.Buffer
	wr := NewWriter(&buf)

	wr.WriteStart("msg-merge")
	wr.WriteData("plan", map[string]string{"step": "1"})
	text := wr.Merge(events)
	wr.WriteData("sources", []string{"https://example.com"})
	wr.WriteFinish()

	output := buf.String()

	if text != "answer" {
		t.Errorf("expected merged text=%q, got %q", "answer", text)
	}
	// start comes before plan
	startIdx := strings.Index(output, `"type":"start"`)
	planIdx := strings.Index(output, `"type":"data-plan"`)
	textIdx := strings.Index(output, `"type":"text-delta"`)
	sourcesIdx := strings.Index(output, `"type":"data-sources"`)
	finishIdx := strings.Index(output, `"type":"finish"`)

	if startIdx < 0 {
		t.Error("missing start chunk")
	}
	if planIdx < 0 {
		t.Error("missing data-plan chunk")
	}
	if textIdx < 0 {
		t.Error("missing text-delta chunk")
	}
	if sourcesIdx < 0 {
		t.Error("missing data-sources chunk")
	}
	if finishIdx < 0 {
		t.Error("missing finish chunk")
	}

	// Order: start < plan < text < sources < finish
	if startIdx >= planIdx {
		t.Error("start should appear before data-plan")
	}
	if planIdx >= textIdx {
		t.Error("data-plan should appear before text-delta")
	}
	if textIdx >= sourcesIdx {
		t.Error("text-delta should appear before data-sources")
	}
	if sourcesIdx >= finishIdx {
		t.Error("data-sources should appear before finish")
	}
}

// TestWriterMerge_ToolResultHook verifies the hook fires during merge.
func TestWriterMerge_ToolResultHook(t *testing.T) {
	events := makeEventStream(
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{
			Type:              aikit.StepEventToolCallStart,
			ToolCallID:        "tc2",
			ToolCallName:      "lookup",
			ToolCallArgsDelta: `{"key":"val"}`,
		},
		aikit.StepEvent{
			Type: aikit.StepEventToolResult,
			ToolResult: &aikit.ToolResult{
				ID:     "tc2",
				Name:   "lookup",
				Args:   `{"key":"val"}`,
				Output: `"found"`,
			},
		},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonToolCalls},
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "result"},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)

	var hookFired bool
	var capturedID string
	hook := func(_ *Writer, result ToolResult) {
		hookFired = true
		capturedID = result.ToolCallID
	}

	var buf bytes.Buffer
	wr := NewWriter(&buf)
	wr.Merge(events, MergeWithToolResultHook(hook))

	if !hookFired {
		t.Error("expected tool result hook to fire")
	}
	if capturedID != "tc2" {
		t.Errorf("expected ToolCallID=tc2, got %q", capturedID)
	}
}

// TestWriterMerge_OnEnd verifies the on-end callback fires.
func TestWriterMerge_OnEnd(t *testing.T) {
	events := makeEventStream(
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "done"},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)

	var endedText string
	var buf bytes.Buffer
	wr := NewWriter(&buf)
	wr.Merge(events, MergeWithOnEnd(func(text string) {
		endedText = text
	}))

	if endedText != "done" {
		t.Errorf("expected on-end text=%q, got %q", "done", endedText)
	}
}
