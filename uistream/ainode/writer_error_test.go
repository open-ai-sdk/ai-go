package ainode

import (
	"bytes"
	"errors"
	"testing"
)

// failWriter models a disconnected client: every write fails.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("client disconnected") }

// TestWriter_SurfacesWriteError proves a disconnected client is observable to the
// caller instead of silently swallowed.
func TestWriter_SurfacesWriteError(t *testing.T) {
	wr := NewWriter(failWriter{})
	if err := wr.WriteStart("m1"); err == nil {
		t.Error("WriteStart should surface the writer error")
	}
	if err := wr.WriteFinishWithReason("stop", nil); err == nil {
		t.Error("WriteFinishWithReason should surface the writer error")
	}
	if err := WriteSSE(failWriter{}, Chunk{Type: ChunkTextDelta, Fields: map[string]any{"delta": "x"}}); err == nil {
		t.Error("WriteSSE should surface the writer error")
	}

	ch := make(chan Chunk, 1)
	ch <- Chunk{Type: ChunkStart}
	close(ch)
	if err := WriteSSEStream(failWriter{}, ch); err == nil {
		t.Error("WriteSSEStream should surface the first write error")
	}
}

// TestWriteSSE_SurfacesMarshalError proves an unserializable field is reported,
// not silently dropped (the old code returned early on marshal failure).
func TestWriteSSE_SurfacesMarshalError(t *testing.T) {
	err := WriteSSE(&bytes.Buffer{}, Chunk{Type: "x", Fields: map[string]any{"bad": make(chan int)}})
	if err == nil {
		t.Error("WriteSSE should surface a marshal error for an unserializable field")
	}
}
