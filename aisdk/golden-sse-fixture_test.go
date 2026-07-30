package aisdk

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// updateGolden regenerates the fixture instead of asserting against it:
//
//	go test ./aisdk/ -run TestGoldenSSEFixture -update-golden
//
// Intended for a deliberate protocol change. Because the fixture is what Phase 03
// replays through the real TypeScript validator, regenerating it without re-running
// that check would let a non-conformant stream become the new expectation.
var updateGolden = flag.Bool("update-golden", false, "rewrite testdata/golden-ui-message-stream.sse")

const goldenPath = "testdata/golden-ui-message-stream.sse"

// goldenScenario is one full assistant turn exercising every chunk family that has a
// real producer, in an order the client actually has to handle:
//
//   - reasoning, closed before the tool boundary
//   - text across a tool boundary, with a NEW block id after it
//   - a server-executed tool: input-start, streamed partial JSON, input-available, output
//   - a provider-executed tool
//   - an approval request, and a denial for a different call
//   - a source reference
//   - a stream-level error, whose text is redacted
//
// The four families with no Eino producer (custom, data-*, source-document,
// reasoning-file) are deliberately absent: shipping fixtures for chunks nothing emits
// would assert a contract no code has to keep.
func goldenScenario() []Chunk {
	leak := &APIError{StatusCode: 503, Message: "org_id=acme project=p1 request_id=req_9"}

	return []Chunk{
		StartChunk("msg-golden-1"),
		StartStep(),

		// Reasoning block, with the signature riding on reasoning-end's metadata
		// rather than being flattened into an empty delta.
		ReasoningStart("r0"),
		ReasoningDeltaChunk("r0", "The user wants two tools. "),
		ReasoningDeltaChunk("r0", "I will call both."),
		ReasoningEnd("r0", WithProviderMetadata(map[string]any{
			"anthropic": map[string]any{"signature": "sig-abc123"},
		})),

		// Text before the tool boundary.
		TextStart("t0"),
		TextDeltaChunk("t0", "Looking that up"),
		TextEnd("t0"),

		// Server-executed tool with streamed input.
		ToolInputStart("call_1", "getWeather", WithProviderExecuted(false)),
		ToolInputDelta("call_1", `{"city":`),
		ToolInputDelta("call_1", `"Hanoi"}`),
		ToolInputAvailable("call_1", "getWeather", map[string]any{"city": "Hanoi"},
			WithProviderExecuted(false)),
		ToolOutputAvailable("call_1", map[string]any{"tempC": 31},
			WithProviderExecuted(false)),

		// Provider-executed tool: no input deltas, since its arguments do not stream.
		ToolInputAvailable("call_2", "web_search", map[string]any{"q": "hanoi weather"},
			WithProviderExecuted(true)),
		ToolOutputAvailable("call_2", "3 results", WithProviderExecuted(true)),
		SourceURL("src_1", "https://example.com/hanoi", WithSourceTitle("Hanoi Weather")),

		// A gated tool: requested, and a different one denied.
		ToolInputAvailable("call_3", "sendEmail", map[string]any{"to": "a@b.c"}),
		ToolApprovalRequest("appr_1", "call_3", WithApprovalSignature("sig-approval-1")),
		ToolOutputDenied("call_4"),

		// Text resumes after the tool boundary — a NEW block id, not a reopened t0.
		TextStart("t1"),
		TextDeltaChunk("t1", "It is 31C in Hanoi."),
		TextEnd("t1"),

		FinishStep(),

		// A provider failure reaching the browser. Only the status survives redaction.
		ErrorChunk(leak),

		FinishChunk(WireFinishToolCalls),
	}
}

// renderGolden frames the scenario exactly as the wire carries it, terminator included.
func renderGolden(t *testing.T) string {
	t.Helper()
	var sb strings.Builder
	sw := NewSSEWriter(&sb)
	for _, c := range goldenScenario() {
		if err := sw.WriteChunk(c); err != nil {
			t.Fatalf("WriteChunk %s: %v", c.Type, err)
		}
	}
	if err := sw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return sb.String()
}

// TestGoldenSSEFixture pins the exact bytes. Phase 03 feeds this file to the real
// uiMessageChunkSchema, so any drift here is a protocol change that must be validated
// against the client, not just re-recorded.
func TestGoldenSSEFixture(t *testing.T) {
	got := renderGolden(t)

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s (%d bytes) — re-run the Phase 03 conformance harness", goldenPath, len(got))
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read %s: %v (regenerate with -update-golden)", goldenPath, err)
	}
	if got != string(want) {
		t.Errorf("golden SSE drift.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestGoldenSSEFixture_StructuralInvariants asserts the properties that make the
// fixture worth having, so a regenerated file that broke them still fails.
func TestGoldenSSEFixture_StructuralInvariants(t *testing.T) {
	out := renderGolden(t)
	lines := strings.Split(strings.TrimSuffix(out, "\n\n"), "\n\n")

	// Terminator: exactly once, and last.
	if n := strings.Count(out, "data: [DONE]"); n != 1 {
		t.Errorf("[DONE] count = %d, want 1", n)
	}
	if lines[len(lines)-1] != "data: [DONE]" {
		t.Errorf("last frame = %q, want the terminator", lines[len(lines)-1])
	}

	// Every frame is a data: line — no stray bytes between frames.
	for i, l := range lines {
		if !strings.HasPrefix(l, "data: ") {
			t.Errorf("frame %d is not an SSE data line: %q", i, l)
		}
	}

	// No *-delta precedes its *-start. This is one of the seven client throw sites.
	idx := func(s string) int { return strings.Index(out, s) }
	for _, pair := range [][2]string{
		{`"type":"text-start"`, `"type":"text-delta"`},
		{`"type":"reasoning-start"`, `"type":"reasoning-delta"`},
		{`"type":"tool-input-start"`, `"type":"tool-input-delta"`},
	} {
		start, delta := idx(pair[0]), idx(pair[1])
		if start < 0 || delta < 0 {
			t.Errorf("fixture lost %s or %s", pair[0], pair[1])
			continue
		}
		if start > delta {
			t.Errorf("%s precedes %s — the client throws on this", pair[1], pair[0])
		}
	}

	// Text resumes under a new block id after the tool boundary. A reused id would
	// silently overwrite the earlier part rather than appending a new one.
	if strings.Count(out, `"id":"t0"`) == 0 || strings.Count(out, `"id":"t1"`) == 0 {
		t.Error("expected two distinct text block ids across the tool boundary")
	}

	// The reasoning signature survives as providerMetadata, not as an empty delta.
	if !strings.Contains(out, "sig-abc123") {
		t.Error("reasoning signature missing from the stream")
	}
	if strings.Contains(out, `"delta":""`) {
		t.Error("an empty delta was emitted; a signature must not be flattened into one")
	}

	// Redaction holds on the wire, not just in the constructor.
	for _, secret := range []string{"acme", "p1", "req_9"} {
		if strings.Contains(out, secret) {
			t.Errorf("provider error detail %q leaked into the stream", secret)
		}
	}
	if !strings.Contains(out, "503") {
		t.Error("status code should survive redaction")
	}

	// finishReason is a wire value.
	if !strings.Contains(out, `"finishReason":"tool-calls"`) {
		t.Error("finishReason is not the hyphenated wire form")
	}
	if strings.Contains(out, "tool_calls") {
		t.Error("an underscore finish reason reached the wire")
	}

	// Every toolCallId is unique per tool-input-start — the client does NOT check this
	// (its lookup falls back to a reverse scan), so it has to be a producer invariant.
	seen := map[string]bool{}
	for _, c := range goldenScenario() {
		if c.Type != ChunkToolInputStart {
			continue
		}
		id, _ := c.Fields["toolCallId"].(string)
		if id == "" {
			t.Error("tool-input-start with an empty toolCallId")
		}
		if seen[id] {
			t.Errorf("toolCallId %q reused within one message", id)
		}
		seen[id] = true
	}
}

// TestGoldenScenario_NoUnproducibleChunkFamilies documents the deliberate omission, so
// a future contributor adding a custom or data-* fixture has to decide consciously.
func TestGoldenScenario_NoUnproducibleChunkFamilies(t *testing.T) {
	out := renderGolden(t)
	for _, absent := range []string{`"type":"custom"`, `"type":"data-`, `"type":"source-document"`, `"type":"reasoning-file"`} {
		if strings.Contains(out, absent) {
			t.Errorf("fixture contains %s, which no Eino content block produces; "+
				"either wire up a producer or leave it out of the golden stream", absent)
		}
	}
	// Sanity: the redaction path is exercised, so ErrorChunk stays covered.
	if !strings.Contains(out, `"type":"error"`) {
		t.Error("fixture no longer exercises the error path")
	}
}
