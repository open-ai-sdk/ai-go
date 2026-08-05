// Pin the contract that a tool result carrying the error disposition reaches
// the client as tool-output-error with a usable errorText, and that every other
// disposition still produces tool-output-available.
package ainode

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func TestChunksToolResult_ErrorDispositionEmitsOutputError(t *testing.T) {
	cp := NewChunkProducer("msg-1")
	chunks := cp.chunksToolResult(aikit.StepEvent{
		Type: aikit.StepEventToolResult,
		ToolResult: &aikit.ToolResult{
			ID: "call_1", Name: "lookup_order", Args: `{"id":"42"}`,
			Output:      "Tool execution failed.",
			Error:       errors.New(`dial tcp 10.0.3.14:8080: token=sk-live-abc`),
			Disposition: aikit.ToolResultError,
		},
	})

	out := requireChunkOfType(t, chunks, ChunkToolOutputError)
	// Output is the scrubbed text the engine built with tool.Details; Error
	// still holds the raw chain. The wire must carry the scrubbed one.
	if got := out.Fields["errorText"]; got != "Tool execution failed." {
		t.Errorf("errorText = %v; want the scrubbed model-visible output", got)
	}
	if got, _ := out.Fields["errorText"].(string); strings.Contains(got, "sk-live-abc") {
		t.Errorf("errorText leaked the raw error chain: %q", got)
	}
	if out.Fields["output"] != nil {
		t.Errorf("output = %v; want absent on the error chunk", out.Fields["output"])
	}
	for _, c := range chunks {
		if c.Type == ChunkToolOutputAvailable {
			t.Fatal("error disposition also emitted tool-output-available")
		}
	}
}

// The client cannot render an input-less tool part, so the back-fill has to run
// on the error path too.
func TestChunksToolResult_ErrorDispositionStillBackfillsInput(t *testing.T) {
	cp := NewChunkProducer("msg-1")
	chunks := cp.chunksToolResult(aikit.StepEvent{
		Type: aikit.StepEventToolResult,
		ToolResult: &aikit.ToolResult{
			ID: "call_1", Name: "lookup_order", Args: `{"id":"42"}`,
			Error: errors.New("boom"), Disposition: aikit.ToolResultError,
		},
	})

	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2 (input back-fill + output-error)", len(chunks))
	}
	if chunks[0].Type != ChunkToolInputAvailable {
		t.Fatalf("chunks[0].Type = %q, want %q", chunks[0].Type, ChunkToolInputAvailable)
	}
	parsed, ok := chunks[0].Fields["input"].(map[string]any)
	if !ok {
		t.Fatalf("input field was %T, want map[string]any", chunks[0].Fields["input"])
	}
	if parsed["id"] != "42" {
		t.Errorf("input.id = %v, want 42", parsed["id"])
	}
}

// A caller that builds a ToolResult directly may set only the error. An empty
// errorText would render as a blank failure card.
func TestChunksToolResult_ErrorDispositionFallsBackToError(t *testing.T) {
	cp := NewChunkProducer("msg-1")
	chunks := cp.chunksToolResult(aikit.StepEvent{
		Type: aikit.StepEventToolResult,
		ToolResult: &aikit.ToolResult{
			ID: "call_1", Name: "lookup_order", Args: `{}`,
			Error: errors.New("upstream timeout"), Disposition: aikit.ToolResultError,
		},
	})

	out := requireChunkOfType(t, chunks, ChunkToolOutputError)
	if got := out.Fields["errorText"]; got != "upstream timeout" {
		t.Errorf("errorText = %v; want the error fallback", got)
	}
}

func TestChunksToolResult_NonErrorDispositionsKeepOutputAvailable(t *testing.T) {
	for _, disposition := range []aikit.ToolResultDisposition{
		"", aikit.ToolResultSuccess, aikit.ToolResultDenied,
		aikit.ToolResultRefused, aikit.ToolResultSkipped,
	} {
		t.Run(string(disposition), func(t *testing.T) {
			cp := NewChunkProducer("msg-1")
			chunks := cp.chunksToolResult(aikit.StepEvent{
				Type: aikit.StepEventToolResult,
				ToolResult: &aikit.ToolResult{
					ID: "call_1", Name: "echo", Args: `{}`,
					Output: "ok", Disposition: disposition,
				},
			})

			out := requireChunkOfType(t, chunks, ChunkToolOutputAvailable)
			if out.Fields["output"] != "ok" {
				t.Errorf("output = %v; want %q", out.Fields["output"], "ok")
			}
		})
	}
}

func TestChunksToolResult_ErrorTextSurvivesSSESerialization(t *testing.T) {
	cp := NewChunkProducer("msg-1")
	chunks := cp.chunksToolResult(aikit.StepEvent{
		Type: aikit.StepEventToolResult,
		ToolResult: &aikit.ToolResult{
			ID: "call_1", Name: "lookup_order", Args: `{}`,
			Error: errors.New("order 42 not found"), Disposition: aikit.ToolResultError,
		},
	})

	var output bytes.Buffer
	for _, c := range chunks {
		if err := WriteSSE(&output, c); err != nil {
			t.Fatal(err)
		}
	}
	got := output.String()
	if !strings.Contains(got, `"type":"tool-output-error"`) {
		t.Fatalf("no tool-output-error on the wire: %s", got)
	}
	if !strings.Contains(got, "order 42 not found") {
		t.Fatalf("errorText was redacted on the wire: %s", got)
	}
}

func requireChunkOfType(t *testing.T, chunks []Chunk, want string) Chunk {
	t.Helper()
	for _, c := range chunks {
		if c.Type == want {
			return c
		}
	}
	t.Fatalf("no %s in %d chunks", want, len(chunks))
	return Chunk{}
}
