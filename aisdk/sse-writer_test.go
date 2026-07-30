package aisdk

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// countingWriter records both bytes written and Flush calls, so flush-per-chunk is an
// assertion rather than an assumption. Without a flush the client sees nothing until
// the handler returns, which silently turns a stream into a single blocking response —
// a failure no output comparison can detect.
type countingWriter struct {
	httptest.ResponseRecorder
	flushes int
}

func newCountingWriter() *countingWriter {
	return &countingWriter{ResponseRecorder: *httptest.NewRecorder()}
}

func (c *countingWriter) Flush() { c.flushes++ }

var _ http.Flusher = (*countingWriter)(nil)

func TestSSEWriter_FlushesOncePerChunk(t *testing.T) {
	cw := newCountingWriter()
	sw := NewSSEWriter(cw)

	chunks := []Chunk{
		StartChunk("m1"), StartStep(),
		TextStart("t0"), TextDeltaChunk("t0", "a"), TextDeltaChunk("t0", "b"), TextEnd("t0"),
		FinishStep(), FinishChunk(WireFinishStop),
	}
	for _, c := range chunks {
		if err := sw.WriteChunk(c); err != nil {
			t.Fatalf("WriteChunk %s: %v", c.Type, err)
		}
	}
	if cw.flushes != len(chunks) {
		t.Errorf("flushes = %d, want %d (one per chunk)", cw.flushes, len(chunks))
	}

	if err := sw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Close writes [DONE], which is also flushed.
	if cw.flushes != len(chunks)+1 {
		t.Errorf("flushes after Close = %d, want %d", cw.flushes, len(chunks)+1)
	}
}

func TestSSEWriter_DoneEmittedExactlyOnce(t *testing.T) {
	cw := newCountingWriter()
	sw := NewSSEWriter(cw)

	if err := sw.WriteChunk(TextDeltaChunk("t0", "hi")); err != nil {
		t.Fatal(err)
	}
	// A finish chunk must NOT itself terminate the stream — that is Close's job.
	if err := sw.WriteChunk(FinishChunk(WireFinishStop)); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(cw.Body.String(), "[DONE]"); n != 0 {
		t.Errorf("[DONE] appeared %d times before Close; want 0", n)
	}

	_ = sw.Close()
	_ = sw.Close() // idempotent
	_ = sw.Close()

	if n := strings.Count(cw.Body.String(), "data: [DONE]\n\n"); n != 1 {
		t.Errorf("[DONE] count = %d after three Close calls, want 1\nbody:\n%s",
			n, cw.Body.String())
	}
}

// TestSSEWriter_DoneEmittedOnErrorPath is the case the old finish-triggered
// terminator could not handle: a run that fails emits `error`, never `finish`, so the
// stream would never terminate and the client would wait on a dead stream.
func TestSSEWriter_DoneEmittedOnErrorPath(t *testing.T) {
	cw := newCountingWriter()
	sw := NewSSEWriter(cw)

	if err := sw.WriteChunk(TextDeltaChunk("t0", "partial")); err != nil {
		t.Fatal(err)
	}
	if err := sw.WriteChunk(ErrorChunk(errors.New("provider exploded"))); err != nil {
		t.Fatal(err)
	}
	_ = sw.Close()

	body := cw.Body.String()
	if !strings.Contains(body, `"type":"error"`) {
		t.Errorf("error chunk missing:\n%s", body)
	}
	if strings.Contains(body, `"type":"finish"`) {
		t.Errorf("no finish chunk was written, yet one appeared:\n%s", body)
	}
	if n := strings.Count(body, "data: [DONE]\n\n"); n != 1 {
		t.Errorf("[DONE] count = %d on the error path, want 1\n%s", n, body)
	}
}

// TestSSEWriter_DoneEmittedOnAbortPath — same for abort, which also has no finish.
func TestSSEWriter_DoneEmittedOnAbortPath(t *testing.T) {
	cw := newCountingWriter()
	sw := NewSSEWriter(cw)
	_ = sw.WriteChunk(AbortChunk("client disconnected"))
	_ = sw.Close()
	if n := strings.Count(cw.Body.String(), "data: [DONE]\n\n"); n != 1 {
		t.Errorf("[DONE] count = %d on the abort path, want 1", n)
	}
}

func TestSSEWriter_WriteAfterCloseRejected(t *testing.T) {
	sw := NewSSEWriter(newCountingWriter())
	_ = sw.Close()
	if err := sw.WriteChunk(TextDeltaChunk("t0", "late")); !errors.Is(err, ErrStreamClosed) {
		t.Errorf("WriteChunk after Close err = %v, want ErrStreamClosed", err)
	}
}

// failingWriter fails every write, standing in for a disconnected client.
type failingWriter struct{ n int }

func (f *failingWriter) Write(p []byte) (int, error) {
	f.n++
	return 0, errors.New("broken pipe")
}

func TestSSEWriter_LatchesFirstWriteError(t *testing.T) {
	fw := &failingWriter{}
	sw := NewSSEWriter(fw)

	if err := sw.WriteChunk(TextDeltaChunk("t0", "a")); err == nil {
		t.Fatal("expected a write error")
	}
	// Still callable, so the producer is not forced to branch on every write.
	_ = sw.WriteChunk(TextDeltaChunk("t0", "b"))

	if err := sw.Err(); err == nil {
		t.Error("Err() is nil after a failed write")
	}
	// Close reports the stream's first error, so a deferred Close is enough to learn
	// the client went away.
	if err := sw.Close(); err == nil {
		t.Error("Close() did not report the latched write error")
	}
}

// TestSSEWriter_FrameFormat pins the exact framing the client's SSE parser expects.
func TestSSEWriter_FrameFormat(t *testing.T) {
	cw := newCountingWriter()
	sw := NewSSEWriter(cw)
	_ = sw.WriteChunk(TextDeltaChunk("t0", "hi"))
	_ = sw.Close()

	want := "data: {\"delta\":\"hi\",\"id\":\"t0\",\"type\":\"text-delta\"}\n\ndata: [DONE]\n\n"
	if got := cw.Body.String(); got != want {
		t.Errorf("framing mismatch\ngot:  %q\nwant: %q", got, want)
	}
}

// TestSSEWriter_NonFlushableWriterIsAccepted — buffers and tests are not flushable,
// and rejecting them would make the type unusable outside a handler.
func TestSSEWriter_NonFlushableWriterIsAccepted(t *testing.T) {
	var sb strings.Builder
	sw := NewSSEWriter(&sb)
	if err := sw.WriteChunk(StartStep()); err != nil {
		t.Fatalf("WriteChunk: %v", err)
	}
	if err := sw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !strings.Contains(sb.String(), "[DONE]") {
		t.Error("stream not terminated on a non-flushable writer")
	}
}

func TestNewSSEResponseWriter_SetsHeadersAndFlushes(t *testing.T) {
	cw := newCountingWriter()
	sw := NewSSEResponseWriter(cw)

	for name, want := range StreamHeaders() {
		if got := cw.Header().Get(name); got != want {
			t.Errorf("header %s = %q, want %q", name, got, want)
		}
	}
	// Headers are flushed before any chunk, so the client sees a streaming response
	// immediately rather than after the first token.
	if cw.flushes < 1 {
		t.Error("headers were not flushed")
	}
	_ = sw.Close()
}

func TestSetStreamHeaders_ExactSetAndIdempotent(t *testing.T) {
	h := http.Header{}
	SetStreamHeaders(h)
	SetStreamHeaders(h) // must not duplicate values

	want := map[string]string{
		"content-type":                  "text/event-stream",
		"cache-control":                 "no-cache",
		"connection":                    "keep-alive",
		"x-vercel-ai-ui-message-stream": "v1",
		"x-accel-buffering":             "no",
	}
	if len(h) != len(want) {
		t.Errorf("header count = %d, want %d: %v", len(h), len(want), h)
	}
	for name, value := range want {
		vs := h.Values(name)
		if len(vs) != 1 {
			t.Errorf("header %s has %d values, want 1: %v", name, len(vs), vs)
		}
		if h.Get(name) != value {
			t.Errorf("header %s = %q, want %q", name, h.Get(name), value)
		}
	}
}
