package aisdk

import (
	"io"
)

// ExecuteFunc is the callback that writes chunks to the stream.
// The Writer is pre-configured; lifecycle (start/finish) is managed by Execute.
type ExecuteFunc func(w *Writer) error

// StreamOptions configures a UI message stream execution.
type StreamOptions struct {
	// MessageID is the assistant message identifier emitted in the start chunk.
	MessageID string

	// Metadata is optional message-level metadata emitted alongside the start chunk.
	Metadata any

	// OnEnd is called after the stream completes with the finish reason ("stop" or "error").
	OnEnd func(finishReason string)
}

// Execute runs a UI message stream with managed lifecycle.
// It emits a start chunk, delegates writing to fn, then emits finish + [DONE].
// If fn returns an error, an error chunk is emitted instead of finish.
func Execute(w io.Writer, opts StreamOptions, fn ExecuteFunc) {
	// Emit start chunk. If it fails the client is already gone, so skip fn.
	startFields := map[string]any{"messageId": opts.MessageID}
	if opts.Metadata != nil {
		startFields["messageMetadata"] = opts.Metadata
	}
	if err := WriteSSE(w, Chunk{Type: ChunkStart, Fields: startFields}); err != nil {
		if opts.OnEnd != nil {
			opts.OnEnd("error")
		}
		return
	}

	wr := NewWriter(w)

	// Run the caller-provided execute function.
	err := fn(wr)

	// Emit finish or error; WriteFinish also emits [DONE]. A failed terminal
	// write means the client disconnected, which the finish reason reflects.
	finishReason := "stop"
	var writeErr error
	if err != nil {
		finishReason = "error"
		writeErr = wr.WriteError(err.Error())
	} else {
		writeErr = wr.WriteFinish()
	}
	if writeErr != nil {
		finishReason = "error"
	}

	if opts.OnEnd != nil {
		opts.OnEnd(finishReason)
	}
}
