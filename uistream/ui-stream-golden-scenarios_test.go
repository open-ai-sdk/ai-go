package uistream

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func makeEvents(evs ...aikit.StepEvent) <-chan aikit.StepEvent {
	ch := make(chan aikit.StepEvent, len(evs))
	for _, e := range evs {
		ch <- e
	}
	close(ch)
	return ch
}

func runAdapter(evs ...aikit.StepEvent) string {
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
		aikit.StepEvent{Type: aikit.StepEventStepStart, StepNumber: 0},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "Hello "},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "world"},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
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
		aikit.StepEvent{Type: aikit.StepEventStepStart, StepNumber: 0},
		aikit.StepEvent{Type: aikit.StepEventReasoningDelta, ReasoningDelta: "thinking..."},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "answer"},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
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
		aikit.StepEvent{Type: aikit.StepEventStepStart, StepNumber: 0},
		aikit.StepEvent{
			Type:              aikit.StepEventToolCallStart,
			ToolCallID:        "tc1",
			ToolCallName:      "search",
			ToolCallArgsDelta: `{"q":"go"}`,
		},
		aikit.StepEvent{
			Type:              aikit.StepEventToolCallReady,
			ToolCallID:        "tc1",
			ToolCallName:      "search",
			ToolCallArgsDelta: `{"q":"go"}`,
		},
		aikit.StepEvent{
			Type: aikit.StepEventToolResult,
			ToolResult: &aikit.ToolResult{
				ID:     "tc1",
				Name:   "search",
				Args:   `{"q":"go"}`,
				Output: `{"results":[]}`,
			},
		},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonToolCalls},
		aikit.StepEvent{Type: aikit.StepEventStepStart, StepNumber: 1},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "Found nothing."},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
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
		aikit.StepEvent{Type: aikit.StepEventStepStart, StepNumber: 0},
		aikit.StepEvent{
			Type:              aikit.StepEventToolCallStart,
			ToolCallID:        "tc-slow",
			ToolCallName:      "painter",
			ToolCallArgsDelta: `{"prompt":"cat"}`,
		},
		aikit.StepEvent{
			Type:              aikit.StepEventToolCallReady,
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
		aikit.StepEvent{Type: aikit.StepEventStepStart, StepNumber: 0},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "partial"},
		aikit.StepEvent{Type: aikit.StepEventError, Error: fmt.Errorf("connection reset")},
	)

	assertContains(t, output, `"type":"error"`)
	// Error text is redacted by default — the raw error must not reach the UI.
	assertContains(t, output, "stream error")
	assertNotContains(t, output, "connection reset")
	assertContains(t, output, `"type":"text-end"`)
	assertContains(t, output, `"type":"finish"`)
	assertContains(t, output, `"finishReason":"error"`)
	assertContains(t, output, "data: [DONE]")
}

func TestGolden_ErrorAfterStepEndDoesNotCloseBlockTwice(t *testing.T) {
	tests := []struct {
		name    string
		event   aikit.StepEvent
		endType string
	}{
		{
			name:    "text",
			event:   aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "partial"},
			endType: ChunkTextEnd,
		},
		{
			name:    "reasoning",
			event:   aikit.StepEvent{Type: aikit.StepEventReasoningDelta, ReasoningDelta: "thinking"},
			endType: ChunkReasoningEnd,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := runAdapter(
				aikit.StepEvent{Type: aikit.StepEventStepStart, StepNumber: 0},
				tt.event,
				aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
				aikit.StepEvent{Type: aikit.StepEventError, Error: fmt.Errorf("structured output failed")},
			)

			endChunk := `"type":"` + tt.endType + `"`
			if got := strings.Count(output, endChunk); got != 1 {
				t.Fatalf("%s count = %d, want 1\n%s", endChunk, got, output)
			}
			finishStep := strings.Index(output, `"type":"finish-step"`)
			errorChunk := strings.Index(output, `"type":"error"`)
			finish := strings.Index(output, `"type":"finish"`)
			if finishStep == -1 || errorChunk == -1 || finish == -1 {
				t.Fatalf("missing terminal chunks\n%s", output)
			}
			if finishStep >= errorChunk || errorChunk >= finish {
				t.Fatalf("terminal chunks out of order\n%s", output)
			}
			assertContains(t, output, `"finishReason":"error"`)
			assertContains(t, output, "data: [DONE]")
		})
	}
}

func TestGolden_ToolCallsFinishReasonUsesWireVocabulary(t *testing.T) {
	output := runAdapter(
		aikit.StepEvent{Type: aikit.StepEventStepStart, StepNumber: 0},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonToolCalls},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)

	assertContains(t, output, `"finishReason":"tool-calls"`)
	assertNotContains(t, output, `"finishReason":"tool_calls"`)
}

// --- full text is returned ---

func TestStream_ReturnsFullText(t *testing.T) {
	a := NewAdapter("msg-1")
	var buf bytes.Buffer
	text := a.Stream(makeEvents(
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "Hello "},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "world"},
		aikit.StepEvent{Type: aikit.StepEventStepEnd},
		aikit.StepEvent{Type: aikit.StepEventDone},
	), &buf)

	if text != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", text)
	}
}
