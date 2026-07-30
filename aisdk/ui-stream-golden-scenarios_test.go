package aisdk

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func makeEvents(evs ...StepEvent) <-chan StepEvent {
	ch := make(chan StepEvent, len(evs))
	for _, e := range evs {
		ch <- e
	}
	close(ch)
	return ch
}

func runAdapter(evs ...StepEvent) string {
	a := NewAdapter("msg-test")
	var buf bytes.Buffer
	a.Stream(makeEvents(evs...), &buf)
	return buf.String()
}

func assertContains(t *testing.T, output, want string) {
	t.Helper()
	if !strings.Contains(output, want) {
		t.Errorf("expected output to contain %q\nfull output:\n%s", want, output)
	}
}

func assertNotContains(t *testing.T, output, want string) {
	t.Helper()
	if strings.Contains(output, want) {
		t.Errorf("expected output NOT to contain %q\nfull output:\n%s", want, output)
	}
}

// --- text-only golden ---

func TestGolden_TextOnly(t *testing.T) {
	output := runAdapter(
		StepEvent{Type: StepEventStepStart, StepNumber: 0},
		StepEvent{Type: StepEventTextDelta, TextDelta: "Hello "},
		StepEvent{Type: StepEventTextDelta, TextDelta: "world"},
		StepEvent{Type: StepEventStepEnd, FinishReason: FinishReasonStop},
		StepEvent{Type: StepEventDone},
	)

	assertContains(t, output, `"type":"start"`)
	assertContains(t, output, `"type":"start-step"`)
	assertContains(t, output, `"type":"text-start"`)
	assertContains(t, output, `"type":"text-delta"`)
	assertContains(t, output, `"delta":"Hello "`)
	assertContains(t, output, `"delta":"world"`)
	assertContains(t, output, `"type":"text-end"`)
	assertContains(t, output, `"type":"finish-step"`)
	assertContains(t, output, `"type":"finish"`)
	assertContains(t, output, "[DONE]")
}

// --- reasoning golden ---

func TestGolden_Reasoning(t *testing.T) {
	output := runAdapter(
		StepEvent{Type: StepEventStepStart, StepNumber: 0},
		StepEvent{Type: StepEventReasoningDelta, ReasoningDelta: "thinking..."},
		StepEvent{Type: StepEventTextDelta, TextDelta: "answer"},
		StepEvent{Type: StepEventStepEnd, FinishReason: FinishReasonStop},
		StepEvent{Type: StepEventDone},
	)

	assertContains(t, output, `"type":"reasoning-start"`)
	assertContains(t, output, `"type":"reasoning-delta"`)
	assertContains(t, output, `"delta":"thinking..."`)
	assertContains(t, output, `"type":"reasoning-end"`)
	assertContains(t, output, `"type":"text-start"`)
	assertContains(t, output, `"delta":"answer"`)
}

// --- tool-call golden ---

func TestGolden_ToolCall(t *testing.T) {
	output := runAdapter(
		StepEvent{Type: StepEventStepStart, StepNumber: 0},
		StepEvent{
			Type:              StepEventToolCallStart,
			ToolCallID:        "tc1",
			ToolCallName:      "search",
			ToolCallArgsDelta: `{"q":"go"}`,
		},
		StepEvent{
			Type:              StepEventToolCallReady,
			ToolCallID:        "tc1",
			ToolCallName:      "search",
			ToolCallArgsDelta: `{"q":"go"}`,
		},
		StepEvent{
			Type: StepEventToolResult,
			ToolResult: &StepToolResult{
				ID:     "tc1",
				Name:   "search",
				Args:   `{"q":"go"}`,
				Output: `{"results":[]}`,
			},
		},
		StepEvent{Type: StepEventStepEnd, FinishReason: FinishReasonToolCalls},
		StepEvent{Type: StepEventStepStart, StepNumber: 1},
		StepEvent{Type: StepEventTextDelta, TextDelta: "Found nothing."},
		StepEvent{Type: StepEventStepEnd, FinishReason: FinishReasonStop},
		StepEvent{Type: StepEventDone},
	)

	assertContains(t, output, `"type":"tool-input-start"`)
	assertContains(t, output, `"toolCallId":"tc1"`)
	assertContains(t, output, `"toolName":"search"`)
	assertContains(t, output, `"type":"tool-input-delta"`)
	assertContains(t, output, `"type":"tool-input-available"`)
	assertContains(t, output, `"type":"tool-output-available"`)
	assertContains(t, output, `"type":"finish-step"`)
	assertContains(t, output, "[DONE]")

	inputIdx := strings.Index(output, `"type":"tool-input-available"`)
	outputIdx := strings.Index(output, `"type":"tool-output-available"`)
	if inputIdx == -1 || outputIdx == -1 || inputIdx > outputIdx {
		t.Fatalf("tool-input-available must be emitted before tool-output-available\n%s", output)
	}
}

func TestGolden_ToolCallReadyEmitsRunningInputBeforeSlowResult(t *testing.T) {
	output := runAdapter(
		StepEvent{Type: StepEventStepStart, StepNumber: 0},
		StepEvent{
			Type:              StepEventToolCallStart,
			ToolCallID:        "tc-slow",
			ToolCallName:      "painter",
			ToolCallArgsDelta: `{"prompt":"cat"}`,
		},
		StepEvent{
			Type:              StepEventToolCallReady,
			ToolCallID:        "tc-slow",
			ToolCallName:      "painter",
			ToolCallArgsDelta: `{"prompt":"cat"}`,
		},
	)

	assertContains(t, output, `"type":"tool-input-available"`)
	assertContains(t, output, `"toolName":"painter"`)
	assertContains(t, output, `"prompt":"cat"`)
	assertNotContains(t, output, `"type":"tool-output-available"`)
}

// --- error golden ---

func TestGolden_Error(t *testing.T) {
	output := runAdapter(
		StepEvent{Type: StepEventStepStart, StepNumber: 0},
		StepEvent{Type: StepEventTextDelta, TextDelta: "partial"},
		StepEvent{Type: StepEventError, Error: fmt.Errorf("connection reset")},
	)

	assertContains(t, output, `"type":"error"`)
	// Error text is redacted by default — the raw error must not reach the UI.
	assertContains(t, output, "stream error")
	assertNotContains(t, output, "connection reset")
	// finish should NOT appear after an error
	assertNotContains(t, output, `"type":"finish"`)
}

// --- full text is returned ---

func TestStream_ReturnsFullText(t *testing.T) {
	a := NewAdapter("msg-1")
	var buf bytes.Buffer
	text := a.Stream(makeEvents(
		StepEvent{Type: StepEventStepStart},
		StepEvent{Type: StepEventTextDelta, TextDelta: "Hello "},
		StepEvent{Type: StepEventTextDelta, TextDelta: "world"},
		StepEvent{Type: StepEventStepEnd},
		StepEvent{Type: StepEventDone},
	), &buf)

	if text != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", text)
	}
}
