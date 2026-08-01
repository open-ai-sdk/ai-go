// Pin block-id uniqueness across every text↔reasoning↔tool boundary.
//
// Mirrors ai-sdk-node's behavior: each conceptual content block (text /
// reasoning / tool_use) is emitted with a unique id. ai-sdk-node uses
// Anthropic's `content_block_index`; ai-go uses an internal counter
// (textBlockCount) that must advance at every boundary.
//
// Without per-boundary advancement, downstream UIs that key parts by id
// (find-or-merge by id) collapse consecutive same-type blocks into one part
// — e.g. text → tool → text shows up as a single text part with the tool
// rendered after, instead of [text, tool, text] in chronological order.
package aisdk

import (
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func TestBlockID_TextReasoningTextHaveDistinctIDs(t *testing.T) {
	cp := NewChunkProducer("msg-1")

	events := []aikit.StepEvent{
		{Type: aikit.StepEventStepStart},
		{Type: aikit.StepEventTextDelta, TextDelta: "Hi."},
		{Type: aikit.StepEventReasoningDelta, ReasoningDelta: "Let me think."},
		{Type: aikit.StepEventTextDelta, TextDelta: "Done."},
		{Type: aikit.StepEventStepEnd},
	}

	var allChunks []Chunk
	for _, ev := range events {
		chunks, _ := cp.translateEvent(ev)
		allChunks = append(allChunks, chunks...)
	}

	textIDs, reasoningIDs := collectStartIDs(allChunks)
	if len(textIDs) != 2 {
		t.Fatalf("got %d text-start, want 2; chunks=%v", len(textIDs), allChunks)
	}
	if textIDs[0] == textIDs[1] {
		t.Errorf("text segments share id %q; must differ", textIDs[0])
	}
	if len(reasoningIDs) != 1 {
		t.Fatalf("got %d reasoning-start, want 1; chunks=%v", len(reasoningIDs), allChunks)
	}
	if reasoningIDs[0] == textIDs[0] || reasoningIDs[0] == textIDs[1] {
		t.Errorf("reasoning id %q collides with a text id; want distinct ids per block",
			reasoningIDs[0])
	}

	// text→reasoning boundary must close text first.
	mustContainSubsequence(t, chunkTypes(allChunks), []string{
		ChunkTextStart, ChunkTextEnd, ChunkReasoningStart, ChunkReasoningEnd, ChunkTextStart, ChunkTextEnd,
	})
}

func TestBlockID_ReasoningToolReasoningHaveDistinctIDs(t *testing.T) {
	cp := NewChunkProducer("msg-1")

	events := []aikit.StepEvent{
		{Type: aikit.StepEventStepStart},
		{Type: aikit.StepEventReasoningDelta, ReasoningDelta: "Plan A."},
		{
			Type:              aikit.StepEventToolCallStart,
			ToolCallID:        "call_1",
			ToolCallName:      "lookup",
			ToolCallArgsDelta: `{"q":"x"}`,
		},
		{Type: aikit.StepEventReasoningDelta, ReasoningDelta: "Refined plan."},
	}

	var allChunks []Chunk
	for _, ev := range events {
		chunks, _ := cp.translateEvent(ev)
		allChunks = append(allChunks, chunks...)
	}

	_, reasoningIDs := collectStartIDs(allChunks)
	if len(reasoningIDs) != 2 {
		t.Fatalf("got %d reasoning-start, want 2 (one before tool, one after); chunks=%v",
			len(reasoningIDs), allChunks)
	}
	if reasoningIDs[0] == reasoningIDs[1] {
		t.Errorf("reasoning segments share id %q across tool boundary; must differ",
			reasoningIDs[0])
	}

	// reasoning→tool boundary must close reasoning first.
	mustContainSubsequence(t, chunkTypes(allChunks), []string{
		ChunkReasoningStart, ChunkReasoningEnd, ChunkToolInputStart, ChunkReasoningStart,
	})
}

func TestBlockID_TextReasoningBoundaryClosesText(t *testing.T) {
	// Specific check for the text→reasoning boundary: text-end must be
	// emitted before reasoning-start (symmetric to the existing
	// reasoning→text behavior).
	cp := NewChunkProducer("msg-1")
	cp.translateEvent(aikit.StepEvent{Type: aikit.StepEventStepStart})
	cp.translateEvent(aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "Hello."})
	textIDBefore := cp.textBlockID

	chunks := cp.chunksReasoningDelta(aikit.StepEvent{
		Type:           aikit.StepEventReasoningDelta,
		ReasoningDelta: "Reflecting...",
	})

	if len(chunks) < 2 {
		t.Fatalf("got %d chunks, want at least 2 (text-end + reasoning-start); chunks=%v",
			len(chunks), chunks)
	}
	if chunks[0].Type != ChunkTextEnd {
		t.Errorf("chunks[0].Type = %q; want %q (text must close before reasoning)",
			chunks[0].Type, ChunkTextEnd)
	}
	if id, _ := chunks[0].Fields["id"].(string); id != textIDBefore {
		t.Errorf("text-end id = %q; want %q", id, textIDBefore)
	}
	if cp.textStarted {
		t.Error("textStarted must be reset after emitting text-end")
	}
	if cp.textBlockID == textIDBefore {
		t.Errorf("textBlockID = %q; must advance after closing text", cp.textBlockID)
	}
}

// collectStartIDs returns the ids found on text-start and reasoning-start chunks.
func collectStartIDs(chunks []Chunk) (textIDs, reasoningIDs []string) {
	for _, c := range chunks {
		switch c.Type {
		case ChunkTextStart:
			if id, ok := c.Fields["id"].(string); ok {
				textIDs = append(textIDs, id)
			}
		case ChunkReasoningStart:
			if id, ok := c.Fields["id"].(string); ok {
				reasoningIDs = append(reasoningIDs, id)
			}
		}
	}
	return textIDs, reasoningIDs
}

func chunkTypes(chunks []Chunk) []string {
	types := make([]string, 0, len(chunks))
	for _, c := range chunks {
		types = append(types, c.Type)
	}
	return types
}

func mustContainSubsequence(t *testing.T, got, want []string) {
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
