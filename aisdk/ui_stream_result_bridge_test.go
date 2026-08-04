package aisdk

import (
	"bytes"
	"iter"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func makeEventStream(events ...aikit.StepEvent) iter.Seq2[aikit.StepEvent, error] {
	return newEventStream(events...)
}

// TestStreamToWriter_BasicTextStream verifies SSE output contains expected chunks.
func TestStreamToWriter_BasicTextStream(t *testing.T) {
	events := makeEventStream(
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "Hello "},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "world"},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)

	var buf bytes.Buffer
	text := StreamToWriter(events, &buf, "msg-1")
	output := buf.String()

	if text != "Hello world" {
		t.Errorf("expected text=%q, got %q", "Hello world", text)
	}
	assertContains(t, output, `"type":"start"`)
	assertContains(t, output, `"messageId":"msg-1"`)
	assertContains(t, output, `"type":"text-delta"`)
	assertContains(t, output, `"delta":"Hello "`)
	assertContains(t, output, `"delta":"world"`)
	assertContains(t, output, `"type":"finish"`)
	assertContains(t, output, "[DONE]")
}

// TestStreamToWriter_ToolResultHookFires verifies the tool result hook is invoked.
func TestStreamToWriter_ToolResultHookFires(t *testing.T) {
	events := makeEventStream(
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{
			Type:              aikit.StepEventToolCallStart,
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
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "done"},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)

	var hookFired bool
	var capturedResult ToolResult
	hook := func(_ *Writer, result ToolResult) {
		hookFired = true
		capturedResult = result
	}

	var buf bytes.Buffer
	StreamToWriter(events, &buf, "msg-hook", WithUIToolResultHook(hook))

	if !hookFired {
		t.Error("expected tool result hook to fire")
	}
	if capturedResult.ToolCallID != "tc1" {
		t.Errorf("expected ToolCallID=tc1, got %q", capturedResult.ToolCallID)
	}
	if capturedResult.ToolName != "search" {
		t.Errorf("expected ToolName=search, got %q", capturedResult.ToolName)
	}
	if capturedResult.ArgsJSON != `{"q":"go"}` {
		t.Errorf("expected ArgsJSON=%q, got %q", `{"q":"go"}`, capturedResult.ArgsJSON)
	}
}

// TestStreamToWriter_OnEndCallback verifies the onEnd callback is invoked with full text.
func TestStreamToWriter_OnEndCallback(t *testing.T) {
	events := makeEventStream(
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "hello"},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)

	var ended string
	var buf bytes.Buffer
	StreamToWriter(events, &buf, "msg-finish", WithUIOnEnd(func(text string) {
		ended = text
	}))

	if ended != "hello" {
		t.Errorf("expected onEnd text=%q, got %q", "hello", ended)
	}
}

// TestStreamToWriter_SSELineFormat verifies every line is prefixed with "data: ".
func TestStreamToWriter_SSELineFormat(t *testing.T) {
	events := makeEventStream(
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "x"},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)

	var buf bytes.Buffer
	StreamToWriter(events, &buf, "msg-fmt")

	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			t.Errorf("SSE line missing 'data: ' prefix: %q", line)
		}
	}
}
