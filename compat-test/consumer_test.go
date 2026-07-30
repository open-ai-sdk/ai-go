package compattest

import (
	"strings"
	"testing"
)

// Compilation is the real assertion in this module — it proves an outside caller can
// name and implement the aisdk surface. The runtime checks below confirm the values
// actually flow, so a surface that compiles but produces nothing still fails.

func TestProduceChunks_EmitsLifecycleAndContent(t *testing.T) {
	chunks, err := ProduceChunks()
	if err != nil {
		t.Fatalf("ProduceChunks: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("no chunks produced")
	}

	seen := map[string]int{}
	for _, c := range chunks {
		seen[c.Type]++
	}
	for _, want := range []string{"start-step", "text-start", "text-delta", "text-end", "finish-step"} {
		if seen[want] == 0 {
			t.Errorf("missing %q chunk; got %v", want, seen)
		}
	}
}

func TestSerializeSSE_FramesAsDataLine(t *testing.T) {
	out, err := SerializeSSE()
	if err != nil {
		t.Fatalf("SerializeSSE: %v", err)
	}
	if !strings.HasPrefix(out, "data: ") {
		t.Errorf("want an SSE data line, got %q", out)
	}
	if !strings.HasSuffix(out, "\n\n") {
		t.Errorf("want a blank-line frame terminator, got %q", out)
	}
	if !strings.Contains(out, `"type":"text-delta"`) {
		t.Errorf("chunk type missing from payload: %q", out)
	}
}

func TestWriteViaWriter_EmitsSourceChunk(t *testing.T) {
	out, err := WriteViaWriter()
	if err != nil {
		t.Fatalf("WriteViaWriter: %v", err)
	}
	if !strings.Contains(out, "https://example.com") {
		t.Errorf("source url missing: %q", out)
	}
}
