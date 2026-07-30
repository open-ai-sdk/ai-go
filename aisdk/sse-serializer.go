package aisdk

import (
	"encoding/json"
	"fmt"
	"io"
)

// marshalChunk renders a chunk as the JSON object the client validates.
//
// The type is written last so it cannot be shadowed by a stray "type" key in
// Fields, which would produce a chunk claiming to be something it is not.
//
// Note there is no struct and no `omitempty` anywhere in this path: presence is
// decided by whether a key is in Fields. That matters because the protocol
// distinguishes absent from null and from false for several optional fields
// (providerExecuted, dynamic, transient, preliminary, isAutomatic), and Go's
// omitempty cannot express "present and false". A map makes presence explicit by
// construction, so the constructors in chunk-constructors.go are the only place
// that decides it.
func marshalChunk(c Chunk) ([]byte, error) {
	payload := make(map[string]any, len(c.Fields)+1)
	for k, v := range c.Fields {
		payload[k] = v
	}
	payload["type"] = c.Type

	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("aisdk: marshal %q chunk: %w", c.Type, err)
	}
	return b, nil
}

// WriteSSE serializes a single Chunk to SSE format on w as a "data: <json>\n\n"
// line, plus a trailing "data: [DONE]\n\n" for ChunkFinish. It returns any marshal
// or write error instead of discarding it, so a disconnected client is observable to
// the caller rather than silently burning token spend.
//
// Deprecated in favour of SSEWriter, which terminates the stream from Close instead
// of inferring it from a finish chunk. That distinction matters on the abort and
// error paths, which never emit finish and so never terminate here. This function
// survives because the Writer/UIStreamWriter lifecycle is built around it; the
// producer rewrite collapses the two.
func WriteSSE(w io.Writer, c Chunk) error {
	b, err := marshalChunk(c)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
		return err
	}
	if c.Type == ChunkFinish {
		if _, err := fmt.Fprintf(w, "data: [DONE]\n\n"); err != nil {
			return err
		}
	}
	return nil
}

// WriteSSEStream reads all chunks from the channel and writes each as SSE to w,
// blocking until the channel is closed. It keeps draining after a write failure
// (so the producer never blocks on an unread channel) but returns the first
// error encountered, making a mid-stream client disconnect observable.
func WriteSSEStream(w io.Writer, chunks <-chan Chunk) error {
	var firstErr error
	for c := range chunks {
		if err := WriteSSE(w, c); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
