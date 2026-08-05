// Pin the contract that ChunkToolOutputAvailable carries `output` as the
// raw string returned by the tool.
package ainode

import (
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func TestChunksToolResult_OutputIsRawString_JSONShapedTool(t *testing.T) {
	cp := NewChunkProducer("msg-1")
	envelope := `{"kind":"painter","status":"completed","images":[{"path":"/tmp/x.png"}]}`
	chunks := cp.chunksToolResult(aikit.StepEvent{
		Type: aikit.StepEventToolResult,
		ToolResult: &aikit.ToolResult{
			ID:     "call_1",
			Name:   "painter",
			Args:   `{"prompt":"a cat"}`,
			Output: envelope,
		},
	})

	out := requireToolOutputChunk(t, chunks)
	got, ok := out.Fields["output"].(string)
	if !ok {
		t.Fatalf("output field was %T, want string", out.Fields["output"])
	}
	if got != envelope {
		t.Errorf("output = %q; want %q (raw string passthrough, no parse)", got, envelope)
	}
}

func TestChunksToolResult_OutputIsRawString_PlainTextTool(t *testing.T) {
	cp := NewChunkProducer("msg-1")
	plain := "exit code 0\nhello world"
	chunks := cp.chunksToolResult(aikit.StepEvent{
		Type: aikit.StepEventToolResult,
		ToolResult: &aikit.ToolResult{
			ID:     "call_2",
			Name:   "bash",
			Args:   `{"cmd":"echo hi"}`,
			Output: plain,
		},
	})

	out := requireToolOutputChunk(t, chunks)
	got, ok := out.Fields["output"].(string)
	if !ok {
		t.Fatalf("output field was %T, want string (no {result:...} wrap)", out.Fields["output"])
	}
	if got != plain {
		t.Errorf("output = %q; want %q (plain text passthrough)", got, plain)
	}
}

func TestChunksToolResult_OutputIsRawString_EmptyTool(t *testing.T) {
	cp := NewChunkProducer("msg-1")
	chunks := cp.chunksToolResult(aikit.StepEvent{
		Type: aikit.StepEventToolResult,
		ToolResult: &aikit.ToolResult{
			ID:     "call_3",
			Name:   "noop",
			Args:   `{}`,
			Output: "",
		},
	})

	out := requireToolOutputChunk(t, chunks)
	got, ok := out.Fields["output"].(string)
	if !ok {
		t.Fatalf("output field was %T, want string (empty must still be string)", out.Fields["output"])
	}
	if got != "" {
		t.Errorf("output = %q; want empty string", got)
	}
}

func TestChunksToolResult_InputStillParsedForUIRendering(t *testing.T) {
	// `input` continues to be JSON-parsed because tool args are reliably
	// JSON-emitted by the model and downstream UIs render structured fields.
	// This guards against accidentally regressing input parsing alongside
	// the output passthrough change.
	cp := NewChunkProducer("msg-1")
	chunks := cp.chunksToolResult(aikit.StepEvent{
		Type: aikit.StepEventToolResult,
		ToolResult: &aikit.ToolResult{
			ID:     "call_4",
			Name:   "calculator",
			Args:   `{"a":2,"b":3}`,
			Output: "5",
		},
	})

	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2 (input + output)", len(chunks))
	}
	in := chunks[0]
	if in.Type != ChunkToolInputAvailable {
		t.Fatalf("chunks[0].Type = %q, want %q", in.Type, ChunkToolInputAvailable)
	}
	parsed, ok := in.Fields["input"].(map[string]any)
	if !ok {
		t.Fatalf("input field was %T, want map[string]any (parsed)", in.Fields["input"])
	}
	if got, _ := parsed["a"].(float64); got != 2 {
		t.Errorf("input.a = %v, want 2", parsed["a"])
	}
}

func requireToolOutputChunk(t *testing.T, chunks []Chunk) Chunk {
	t.Helper()
	for _, c := range chunks {
		if c.Type == ChunkToolOutputAvailable {
			return c
		}
	}
	t.Fatalf("no ChunkToolOutputAvailable in %d chunks", len(chunks))
	return Chunk{}
}
