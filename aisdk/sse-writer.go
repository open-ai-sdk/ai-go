package aisdk

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// doneFrame terminates a UI message stream. The client's SSE parser treats it as
// end-of-stream, so it must appear exactly once and only at the end.
const doneFrame = "data: [DONE]\n\n"

// ErrStreamClosed is returned by SSEWriter.WriteChunk after Close.
var ErrStreamClosed = errors.New("aisdk: stream already closed")

// SSEWriter frames chunks as Server-Sent Events with the two properties the bare
// WriteSSE helper cannot provide.
//
// First, `[DONE]` is emitted by Close, not by observing a `finish` chunk. Tying it to
// `finish` means an aborted or failed run — which never emits `finish` — never
// terminates the stream either, so the client waits on a stream that is already dead.
// Close runs from the handler's defer, so every exit path terminates the stream
// exactly once.
//
// Second, it flushes after every frame. Both net/http and Gin buffer responses, so
// without an explicit flush the client sees nothing until the handler returns, which
// defeats streaming entirely. x-accel-buffering:no handles nginx, not Go's own writer.
type SSEWriter struct {
	w       io.Writer
	flusher http.Flusher

	mu     sync.Mutex
	closed bool
	// writeErr latches the first write failure. Once the client is gone every
	// subsequent write fails too, and reporting the first is what identifies the
	// cause.
	writeErr error
}

// NewSSEWriter wraps w. If w implements http.Flusher, each frame is flushed.
//
// A non-flushable writer is accepted rather than rejected because tests and buffers
// legitimately are not flushable; only a real response writer needs to be.
func NewSSEWriter(w io.Writer) *SSEWriter {
	sw := &SSEWriter{w: w}
	if f, ok := w.(http.Flusher); ok {
		sw.flusher = f
	}
	return sw
}

// NewSSEResponseWriter wraps an http.ResponseWriter, sets the five protocol headers,
// and flushes them so the client sees a streaming response before the first chunk.
func NewSSEResponseWriter(w http.ResponseWriter) *SSEWriter {
	SetStreamHeaders(w.Header())
	w.WriteHeader(http.StatusOK)
	sw := NewSSEWriter(w)
	sw.flush()
	return sw
}

// WriteChunk marshals and writes one chunk, then flushes.
func (sw *SSEWriter) WriteChunk(c Chunk) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if sw.closed {
		return ErrStreamClosed
	}

	b, err := marshalChunk(c)
	if err != nil {
		// A marshal failure is a producer bug, not a transport failure: the stream
		// is still healthy, so it is not latched into writeErr.
		return err
	}
	return sw.writeFrame(fmt.Sprintf("data: %s\n\n", b))
}

// Close writes the terminating [DONE] frame. Safe to call more than once; only the
// first call emits. Intended for a defer.
//
// It returns the first error seen across the whole stream, so a handler's deferred
// Close is enough to learn that the client disconnected mid-response.
func (sw *SSEWriter) Close() error {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if sw.closed {
		return sw.writeErr
	}
	sw.closed = true
	// Attempted even after an earlier failure: if the connection recovered, the
	// client still needs the terminator. writeFrame keeps the first error.
	_ = sw.writeFrame(doneFrame)
	return sw.writeErr
}

// Err reports the first write error, or nil.
func (sw *SSEWriter) Err() error {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.writeErr
}

// writeFrame writes a pre-framed string and flushes. Caller holds mu.
func (sw *SSEWriter) writeFrame(s string) error {
	if _, err := io.WriteString(sw.w, s); err != nil {
		if sw.writeErr == nil {
			sw.writeErr = err
		}
		return err
	}
	sw.flush()
	return nil
}

func (sw *SSEWriter) flush() {
	if sw.flusher != nil {
		sw.flusher.Flush()
	}
}
