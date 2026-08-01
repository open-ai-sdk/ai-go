// Pin the chronological boundary between text and tool calls.
//
// Models often emit text deltas, then a tool call, then more text deltas
// within the same step (e.g. "Let me check the weather…" → tool call → "It
// looks sunny."). Without an explicit text-end + new text block id at the
// text→tool boundary, every text delta in the step shares the same id, the
// downstream UI's text-delta reducer matches by id and concatenates ALL
// segments into the FIRST text part, and tool calls render after the
// concatenated blob — losing chronological order.
//
// chunksToolCallStart must emit ChunkTextEnd, clear textStarted, and advance
// textBlockID so post-tool text gets a fresh ChunkTextStart with a new id.
package aisdk

import (
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func TestChunksToolCallStart_EndsActiveTextBlock(t *testing.T) {
	cp := NewChunkProducer("msg-1")

	// Open a step + start text — same setup the engine produces.
	cp.translateEvent(aikit.StepEvent{Type: aikit.StepEventStepStart})
	cp.translateEvent(aikit.StepEvent{
		Type:      aikit.StepEventTextDelta,
		TextDelta: "Let me check the weather...",
	})
	textIDBefore := cp.textBlockID

	// Now a tool call starts BEFORE step-end. The producer must emit
	// ChunkTextEnd before ChunkToolInputStart and advance the text block id.
	chunks, _ := cp.translateEvent(aikit.StepEvent{
		Type:              aikit.StepEventToolCallStart,
		ToolCallID:        "call_1",
		ToolCallName:      "weather",
		ToolCallArgsDelta: `{"city":"Hanoi"}`,
	})

	if len(chunks) < 2 {
		t.Fatalf("got %d chunks, want at least 2 (text-end + tool-input-start)", len(chunks))
	}
	if chunks[0].Type != ChunkTextEnd {
		t.Errorf("chunks[0].Type = %q; want %q (text must close before tool call)",
			chunks[0].Type, ChunkTextEnd)
	}
	if id, _ := chunks[0].Fields["id"].(string); id != textIDBefore {
		t.Errorf("text-end id = %q; want %q (must close the active block)", id, textIDBefore)
	}
	if chunks[1].Type != ChunkToolInputStart {
		t.Errorf("chunks[1].Type = %q; want %q", chunks[1].Type, ChunkToolInputStart)
	}
	if cp.textStarted {
		t.Error("textStarted must be reset after emitting text-end")
	}
	if cp.textBlockID == textIDBefore {
		t.Errorf("textBlockID = %q; must advance after closing the active block", cp.textBlockID)
	}
}

func TestChunksToolCallStart_NoTextEndWhenInactive(t *testing.T) {
	// Sanity check: when no text is active, chunksToolCallStart should NOT
	// emit a spurious text-end (only tool-related chunks).
	cp := NewChunkProducer("msg-1")
	cp.translateEvent(aikit.StepEvent{Type: aikit.StepEventStepStart})

	chunks, _ := cp.translateEvent(aikit.StepEvent{
		Type:              aikit.StepEventToolCallStart,
		ToolCallID:        "call_1",
		ToolCallName:      "bash",
		ToolCallArgsDelta: `{"cmd":"ls"}`,
	})

	for _, c := range chunks {
		if c.Type == ChunkTextEnd {
			t.Errorf("unexpected ChunkTextEnd emitted when no text was active")
		}
	}
	if len(chunks) == 0 || chunks[0].Type != ChunkToolInputStart {
		t.Errorf("expected first chunk to be ChunkToolInputStart, got %v", chunks)
	}
}

func TestChunksToolCallStart_PostToolTextHasNewBlockID(t *testing.T) {
	// End-to-end: text → tool → text within one step. The two text segments
	// must use DIFFERENT block ids so downstream consumers render them as
	// separate parts in chronological order [text_A, tool, text_B].
	cp := NewChunkProducer("msg-1")

	events := []aikit.StepEvent{
		{Type: aikit.StepEventStepStart},
		{Type: aikit.StepEventTextDelta, TextDelta: "Checking..."},
		{
			Type:              aikit.StepEventToolCallStart,
			ToolCallID:        "call_1",
			ToolCallName:      "lookup",
			ToolCallArgsDelta: `{"q":"x"}`,
		},
		{Type: aikit.StepEventTextDelta, TextDelta: "Found it."},
	}

	var allChunks []Chunk
	for _, ev := range events {
		chunks, _ := cp.translateEvent(ev)
		allChunks = append(allChunks, chunks...)
	}

	// Collect text-start ids in emission order.
	var textStartIDs []string
	for _, c := range allChunks {
		if c.Type == ChunkTextStart {
			if id, ok := c.Fields["id"].(string); ok {
				textStartIDs = append(textStartIDs, id)
			}
		}
	}
	if len(textStartIDs) != 2 {
		t.Fatalf("got %d text-start chunks, want 2 (one per segment); chunks=%v", len(textStartIDs), allChunks)
	}
	if textStartIDs[0] == textStartIDs[1] {
		t.Errorf("both text segments share id %q; must differ to render as separate parts", textStartIDs[0])
	}

	// Order must be: text-start(A) → text-delta(A) → text-end(A) → tool-input-start → text-start(B) → text-delta(B)
	gotOrder := make([]string, 0, len(allChunks))
	for _, c := range allChunks {
		gotOrder = append(gotOrder, c.Type)
	}
	mustContainInOrder(t, gotOrder, []string{
		ChunkTextStart, ChunkTextDelta, ChunkTextEnd,
		ChunkToolInputStart,
		ChunkTextStart, ChunkTextDelta,
	})
}

// mustContainInOrder asserts that `want` appears as a non-contiguous subsequence of `got`.
func mustContainInOrder(t *testing.T, got, want []string) {
	t.Helper()
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	if i < len(want) {
		t.Errorf("chunk order missing expected subsequence\n got: %v\nwant subsequence: %v", got, want)
	}
}
