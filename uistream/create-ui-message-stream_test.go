package uistream

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/internal/engine"
)

// TestCreateUIMessageStream_BasicLifecycle verifies start → custom data → finish → [DONE].
func TestCreateUIMessageStream_BasicLifecycle(t *testing.T) {
	var buf bytes.Buffer

	CreateUIMessageStream(&buf, CreateUIStreamOptions{
		MessageID: "msg-create-1",
	}, func(sw *UIStreamWriter) error {
		sw.WriteData("plan", map[string]string{"step": "1"})
		return nil
	})

	output := buf.String()
	assertContains(t, output, `"type":"start"`)
	assertContains(t, output, `"messageId":"msg-create-1"`)
	assertContains(t, output, `"type":"data-plan"`)
	assertContains(t, output, `"type":"finish"`)
	assertContains(t, output, `"finishReason":"stop"`)
	assertContains(t, output, "data: [DONE]")

	// Verify order: start < data-plan < finish.
	startIdx := strings.Index(output, `"type":"start"`)
	planIdx := strings.Index(output, `"type":"data-plan"`)
	finishIdx := strings.Index(output, `"type":"finish"`)
	if startIdx >= planIdx {
		t.Error("start should appear before data-plan")
	}
	if planIdx >= finishIdx {
		t.Error("data-plan should appear before finish")
	}
}

// TestCreateUIMessageStream_MergeWithToUIMessageStream verifies model stream merging.
func TestCreateUIMessageStream_MergeWithToUIMessageStream(t *testing.T) {
	var buf bytes.Buffer

	CreateUIMessageStream(&buf, CreateUIStreamOptions{
		MessageID: "msg-create-2",
	}, func(sw *UIStreamWriter) error {
		sw.WriteData("plan", map[string]string{"step": "1"})

		sr := newMockStreamEventer(
			engine.StepEvent{Type: engine.StepEventStepStart},
			engine.StepEvent{Type: engine.StepEventTextDelta, TextDelta: "Hello "},
			engine.StepEvent{Type: engine.StepEventTextDelta, TextDelta: "world"},
			engine.StepEvent{Type: engine.StepEventStepEnd, FinishReason: engine.FinishReasonStop},
			engine.StepEvent{Type: engine.StepEventDone},
		)

		chunks := ToUIMessageStream(sr, "msg-create-2", ToUIStreamOptions{
			SendReasoning: true,
			SendSources:   true,
		})
		sw.Merge(chunks)

		sw.WriteData("sources", []string{"https://example.com"})
		return nil
	})

	output := buf.String()

	// All expected chunks.
	assertContains(t, output, `"type":"start"`)
	assertContains(t, output, `"type":"data-plan"`)
	assertContains(t, output, `"type":"text-delta"`)
	assertContains(t, output, `"delta":"Hello "`)
	assertContains(t, output, `"delta":"world"`)
	assertContains(t, output, `"type":"data-sources"`)
	assertContains(t, output, `"type":"finish"`)
	assertContains(t, output, "data: [DONE]")

	// Verify order: plan < text-delta < sources < finish.
	planIdx := strings.Index(output, `"type":"data-plan"`)
	textIdx := strings.Index(output, `"type":"text-delta"`)
	sourcesIdx := strings.Index(output, `"type":"data-sources"`)
	finishIdx := strings.Index(output, `"type":"finish"`)

	if planIdx >= textIdx {
		t.Error("data-plan should appear before text-delta")
	}
	if textIdx >= sourcesIdx {
		t.Error("text-delta should appear before data-sources")
	}
	if sourcesIdx >= finishIdx {
		t.Error("data-sources should appear before finish")
	}

	// Merged stream's start/finish should NOT appear as duplicate chunks.
	// Count start chunks — should only be one.
	startCount := strings.Count(output, `"type":"start"`)
	if startCount != 1 {
		t.Errorf("expected 1 start chunk, got %d", startCount)
	}
}

// TestCreateUIMessageStream_ErrorHandling verifies error → error chunk before finish.
func TestCreateUIMessageStream_ErrorHandling(t *testing.T) {
	var buf bytes.Buffer

	CreateUIMessageStream(&buf, CreateUIStreamOptions{
		MessageID: "msg-create-3",
	}, func(sw *UIStreamWriter) error {
		sw.WriteData("plan", map[string]string{"step": "1"})
		return fmt.Errorf("connection reset")
	})

	output := buf.String()
	assertContains(t, output, `"type":"error"`)
	// Error text is redacted by default — the raw error must not reach the UI.
	assertContains(t, output, "stream error")
	assertNotContains(t, output, "connection reset")
	assertContains(t, output, `"type":"finish"`)
	assertContains(t, output, `"finishReason":"error"`)

	// Error should appear before finish.
	errIdx := strings.Index(output, `"type":"error"`)
	finishIdx := strings.Index(output, `"type":"finish"`)
	if errIdx >= finishIdx {
		t.Error("error chunk should appear before finish")
	}
}

// TestCreateUIMessageStream_OnEndCallback verifies the callback fires with correct data.
func TestCreateUIMessageStream_OnEndCallback(t *testing.T) {
	var buf bytes.Buffer
	var endResult UIStreamEndResult

	CreateUIMessageStream(&buf, CreateUIStreamOptions{
		MessageID: "msg-create-4",
		OnEnd: func(result UIStreamEndResult) {
			endResult = result
		},
	}, func(sw *UIStreamWriter) error {
		sr := newMockStreamEventer(
			engine.StepEvent{Type: engine.StepEventStepStart},
			engine.StepEvent{Type: engine.StepEventTextDelta, TextDelta: "Hello world"},
			engine.StepEvent{Type: engine.StepEventStepEnd, FinishReason: engine.FinishReasonStop},
			engine.StepEvent{Type: engine.StepEventDone},
		)
		sw.MergeStreamResult(sr, "msg-create-4", ToUIStreamOptions{
			SendReasoning: true,
			SendSources:   true,
		})
		return nil
	})

	if endResult.Text != "Hello world" {
		t.Errorf("expected Text=%q, got %q", "Hello world", endResult.Text)
	}
	if endResult.FinishReason != "stop" {
		t.Errorf("expected FinishReason=stop, got %q", endResult.FinishReason)
	}
}

// TestCreateUIMessageStream_OnErrorCallback verifies custom error message.
func TestCreateUIMessageStream_OnErrorCallback(t *testing.T) {
	var buf bytes.Buffer

	CreateUIMessageStream(&buf, CreateUIStreamOptions{
		MessageID: "msg-create-5",
		OnError: func(err error) string {
			return "custom: " + err.Error()
		},
	}, func(sw *UIStreamWriter) error {
		return fmt.Errorf("bad request")
	})

	output := buf.String()
	assertContains(t, output, "custom: bad request")
	assertContains(t, output, `"finishReason":"error"`)
}

// TestCreateUIMessageStream_MultipleWriteData verifies multiple WriteData calls.
func TestCreateUIMessageStream_MultipleWriteData(t *testing.T) {
	var buf bytes.Buffer

	CreateUIMessageStream(&buf, CreateUIStreamOptions{
		MessageID: "msg-create-6",
	}, func(sw *UIStreamWriter) error {
		sw.WriteData("plan", map[string]string{"step": "1"})
		sw.WriteData("steps", []string{"a", "b", "c"})
		sw.WriteTransientData("progress", map[string]int{"percent": 50})
		return nil
	})

	output := buf.String()
	assertContains(t, output, `"type":"data-plan"`)
	assertContains(t, output, `"type":"data-steps"`)
	assertContains(t, output, `"type":"data-progress"`)
	assertContains(t, output, `"transient":true`)
}

// TestCreateUIMessageStream_MetadataInStart verifies metadata is attached to start chunk.
func TestCreateUIMessageStream_MetadataInStart(t *testing.T) {
	var buf bytes.Buffer

	CreateUIMessageStream(&buf, CreateUIStreamOptions{
		MessageID: "msg-create-7",
		Metadata:  map[string]string{"model": "gpt-4o"},
	}, func(sw *UIStreamWriter) error {
		return nil
	})

	output := buf.String()
	assertContains(t, output, `"messageMetadata"`)
	assertContains(t, output, `"model":"gpt-4o"`)
}

// TestCreateUIMessageStream_MergeMetadataFromFinish verifies messageMetadata from merged finish is emitted.
func TestCreateUIMessageStream_MergeMetadataFromFinish(t *testing.T) {
	var buf bytes.Buffer

	CreateUIMessageStream(&buf, CreateUIStreamOptions{
		MessageID: "msg-create-8",
	}, func(sw *UIStreamWriter) error {
		sr := newMockStreamEventer(
			engine.StepEvent{Type: engine.StepEventStepStart},
			engine.StepEvent{Type: engine.StepEventTextDelta, TextDelta: "answer"},
			engine.StepEvent{Type: engine.StepEventUsage, Usage: &engine.Usage{
				InputTokens: 10, OutputTokens: 5, TotalTokens: 15,
			}},
			engine.StepEvent{Type: engine.StepEventStepEnd, FinishReason: engine.FinishReasonStop},
			engine.StepEvent{Type: engine.StepEventDone},
		)
		chunks := ToUIMessageStream(sr, "msg-create-8", ToUIStreamOptions{
			SendReasoning: true,
			SendSources:   true,
			MessageMetadata: func(info MessageMetadataInfo) map[string]any {
				return map[string]any{"tokens": info.Usage.TotalTokens}
			},
		})
		sw.Merge(chunks)
		return nil
	})

	output := buf.String()
	assertContains(t, output, `"type":"message-metadata"`)
	assertContains(t, output, `"tokens":15`)
}

// TestCreateUIMessageStream_OnEndWithError verifies OnEnd fires with error reason.
func TestCreateUIMessageStream_OnEndWithError(t *testing.T) {
	var buf bytes.Buffer
	var endResult UIStreamEndResult

	CreateUIMessageStream(&buf, CreateUIStreamOptions{
		MessageID: "msg-create-9",
		OnEnd: func(result UIStreamEndResult) {
			endResult = result
		},
	}, func(sw *UIStreamWriter) error {
		return fmt.Errorf("something broke")
	})

	if endResult.FinishReason != "error" {
		t.Errorf("expected FinishReason=error, got %q", endResult.FinishReason)
	}
}
