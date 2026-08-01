package aisdkhttp

import (
	"io"
	"net/http"
	"sync"

	"github.com/open-ai-sdk/ai-go/aisdk"
)

const (
	contentTypeHeader    = "Content-Type"
	cacheControlHeader   = "Cache-Control"
	connectionHeader     = "Connection"
	protocolHeader       = "x-vercel-ai-ui-message-stream"
	accelBufferingHeader = "x-accel-buffering"
)

// SetHeaders applies the five response headers required by the v7 UI message
// stream protocol.
func SetHeaders(header http.Header) {
	header.Set(contentTypeHeader, "text/event-stream")
	header.Set(cacheControlHeader, "no-cache")
	header.Set(connectionHeader, "keep-alive")
	header.Set(protocolHeader, "v1")
	header.Set(accelBufferingHeader, "no")
}

// NewSSEWriter returns a writer that applies the UI message stream headers and
// flushes after every write when the response supports http.Flusher.
func NewSSEWriter(w http.ResponseWriter) io.Writer {
	return newSSEWriter(w, nil)
}

// WriteStream writes chunks as SSE and flushes each frame when supported. It
// returns the first write error after draining the input channel.
func WriteStream(w http.ResponseWriter, chunks <-chan aisdk.Chunk) error {
	return aisdk.WriteSSEStream(NewSSEWriter(w), chunks)
}

type sseWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	onError func()
	once    sync.Once
}

func newSSEWriter(w http.ResponseWriter, onError func()) *sseWriter {
	SetHeaders(w.Header())
	flusher, _ := w.(http.Flusher)
	return &sseWriter{w: w, flusher: flusher, onError: onError}
}

func (w *sseWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if err != nil {
		w.once.Do(func() {
			if w.onError != nil {
				w.onError()
			}
		})
		return n, err
	}
	if w.flusher != nil {
		w.flusher.Flush()
	}
	return n, nil
}
