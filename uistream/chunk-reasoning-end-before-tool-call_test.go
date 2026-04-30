// Pin the chronological boundary between reasoning and tool calls.
//
// Models commonly emit reasoning deltas, then a tool call, then more text.
// Without an explicit reasoning-end at the reasoning→tool boundary, the
// downstream PersistedMessageBuilder appends parts in the order they
// terminate — so reasoning-end fires at chunksStepEnd (after tool-output)
// and the persisted parts list ends up [tool, reasoning, …]. UI
// rehydration then renders the painter card BEFORE the thinking card,
// even though reasoning happened first.
//
// chunksTextDelta already inserts reasoning-end at the reasoning→text
// boundary. This test verifies chunksToolCallStart does the same at the
// reasoning→tool boundary.
package uistream

import (
	"testing"

	"github.com/open-ai-sdk/ai-go/internal/engine"
)

func TestChunksToolCallStart_EndsActiveReasoningBlock(t *testing.T) {
	cp := NewChunkProducer("msg-1")

	// Open a step + start reasoning — same setup the engine produces.
	cp.translateEvent(engine.StepEvent{Type: engine.StepEventStepStart})
	cp.translateEvent(engine.StepEvent{
		Type:           engine.StepEventReasoningDelta,
		ReasoningDelta: "Thinking about the user's request...",
	})

	// Now a tool call starts BEFORE any text or step-end. The producer must
	// emit ChunkReasoningEnd before ChunkToolInputStart.
	chunks, _ := cp.translateEvent(engine.StepEvent{
		Type:              engine.StepEventToolCallStart,
		ToolCallID:        "call_1",
		ToolCallName:      "painter",
		ToolCallArgsDelta: `{"prompt":"a cat"}`,
	})

	if len(chunks) < 2 {
		t.Fatalf("got %d chunks, want at least 2 (reasoning-end + tool-input-start)", len(chunks))
	}
	if chunks[0].Type != ChunkReasoningEnd {
		t.Errorf("chunks[0].Type = %q; want %q (reasoning must close before tool call)",
			chunks[0].Type, ChunkReasoningEnd)
	}
	if chunks[1].Type != ChunkToolInputStart {
		t.Errorf("chunks[1].Type = %q; want %q",
			chunks[1].Type, ChunkToolInputStart)
	}
	if cp.reasoningStarted {
		t.Error("reasoningStarted must be reset after emitting reasoning-end")
	}
}

func TestChunksToolCallStart_NoReasoningEndWhenInactive(t *testing.T) {
	// Sanity check: when no reasoning is active, chunksToolCallStart should
	// emit only tool-related chunks (no spurious reasoning-end).
	cp := NewChunkProducer("msg-1")
	cp.translateEvent(engine.StepEvent{Type: engine.StepEventStepStart})

	chunks, _ := cp.translateEvent(engine.StepEvent{
		Type:              engine.StepEventToolCallStart,
		ToolCallID:        "call_1",
		ToolCallName:      "bash",
		ToolCallArgsDelta: `{"cmd":"ls"}`,
	})

	for _, c := range chunks {
		if c.Type == ChunkReasoningEnd {
			t.Errorf("unexpected ChunkReasoningEnd emitted when no reasoning was active")
		}
	}
	if len(chunks) == 0 || chunks[0].Type != ChunkToolInputStart {
		t.Errorf("expected first chunk to be ChunkToolInputStart, got %v", chunks)
	}
}

func TestChunksToolCallStart_PartOrderInPersistedBuilder(t *testing.T) {
	// End-to-end: feed the full event sequence through ChunkProducer +
	// PersistedMessageBuilder and assert parts come out as [reasoning, tool].
	cp := NewChunkProducer("msg-1")
	builder := NewPersistedMessageBuilder()

	events := []engine.StepEvent{
		{Type: engine.StepEventStepStart},
		{Type: engine.StepEventReasoningDelta, ReasoningDelta: "I should call the painter."},
		{Type: engine.StepEventToolCallStart, ToolCallID: "call_1", ToolCallName: "painter", ToolCallArgsDelta: `{"prompt":"cat"}`},
		{Type: engine.StepEventToolCallReady, ToolCallID: "call_1", ToolCallName: "painter"},
		{Type: engine.StepEventToolResult, ToolResult: &engine.ToolResult{
			ID: "call_1", Name: "painter", Args: `{"prompt":"cat"}`, Output: `{"kind":"painter","status":"completed"}`,
		}},
		{Type: engine.StepEventStepEnd},
	}
	for _, ev := range events {
		chunks, _ := cp.translateEvent(ev)
		for _, c := range chunks {
			builder.ObserveChunk(c)
		}
	}

	parts := builder.parts
	// Find the reasoning and tool indices.
	reasoningIdx, toolIdx := -1, -1
	for i, p := range parts {
		m, ok := p.(map[string]any)
		if !ok {
			continue
		}
		switch m["type"] {
		case "reasoning":
			if reasoningIdx == -1 {
				reasoningIdx = i
			}
		case "tool-invocation":
			if toolIdx == -1 {
				toolIdx = i
			}
		}
	}
	if reasoningIdx == -1 {
		t.Fatalf("reasoning part missing from %v", parts)
	}
	if toolIdx == -1 {
		t.Fatalf("tool-invocation part missing from %v", parts)
	}
	if reasoningIdx > toolIdx {
		t.Errorf("reasoning (idx=%d) appears AFTER tool (idx=%d); want reasoning first", reasoningIdx, toolIdx)
	}
}
