package aisdk

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// WriteSSE serializes a single Chunk to SSE format on w as a "data: <json>\n\n"
// line (plus a trailing "data: [DONE]\n\n" for ChunkFinish). It returns any
// marshal or write error instead of discarding it, so a disconnected client is
// observable to the caller rather than silently burning token spend.
func WriteSSE(w io.Writer, c Chunk) error {
	return writeSSE(w, c, false)
}

func writeSSE(w io.Writer, c Chunk, trustedError bool) error {
	if !ValidChunkType(c.Type) {
		return fmt.Errorf("aisdk: unknown UI message chunk type %q", c.Type)
	}
	payload := make(map[string]any, len(c.Fields)+1)
	for k, v := range c.Fields {
		payload[k] = v
	}
	if (c.Type == ChunkError || c.Type == ChunkToolOutputError) && !trustedError {
		if raw, ok := payload["errorText"].(string); ok {
			if !isRedactedStreamError(raw) {
				payload["errorText"] = redactStreamError(errors.New(raw))
			}
		}
	}
	payload["type"] = c.Type

	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("aisdk: marshal %q chunk: %w", c.Type, err)
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
