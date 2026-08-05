package ainode

import (
	"io"
	"iter"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// CreateUIStreamOptions configures CreateUIMessageStream.
type CreateUIStreamOptions struct {
	MessageID string
	Metadata  any
	OnEnd     func(result UIStreamEndResult)
	OnError   func(err error) string // return custom error message, or "" for default
}

// UIStreamEndResult holds info about the completed stream.
type UIStreamEndResult struct {
	Text         string
	FinishReason string
}

// UIStreamWriter provides write + merge capabilities within a CreateUIMessageStream execute callback.
type UIStreamWriter struct {
	writer     *Writer
	text       string
	lastFinish string
}

// WriteData emits a custom data-* chunk.
func (sw *UIStreamWriter) WriteData(name string, payload any) error {
	return sw.writer.WriteData(name, payload)
}

// WriteTransientData emits a transient custom data-* chunk.
func (sw *UIStreamWriter) WriteTransientData(name string, payload any) error {
	return sw.writer.WriteTransientData(name, payload)
}

// mergeChunks pipes chunks from a ToUIMessageStream output into this stream.
// The merge respects lifecycle: it skips the start chunk from the merged stream
// (since the outer stream already emitted start) and captures the finish reason
// without emitting finish (the outer stream manages finish).
// Merge returns the first write error (client disconnected); writes stop after
// it while text tracking continues so the returned text stays accurate.
func (sw *UIStreamWriter) mergeChunks(chunks <-chan Chunk) error {
	var writeErr error
	for c := range chunks {
		switch c.Type {
		case ChunkStart:
			// Skip — outer stream already emitted start.
		case ChunkFinish:
			if fr, ok := c.Fields["finishReason"].(string); ok {
				sw.lastFinish = fr
			}
			// Emit message-metadata from finish if present.
			if md, ok := c.Fields["messageMetadata"]; ok && md != nil && writeErr == nil {
				writeErr = sw.writer.WriteMessageMetadata(md)
			}
			// Don't emit finish — outer manages lifecycle.
		default:
			if writeErr == nil {
				writeErr = sw.writer.writeChunk(c)
			}
			// Track text for finish result.
			if c.Type == ChunkTextDelta {
				if delta, ok := c.Fields["delta"].(string); ok {
					sw.text += delta
				}
			}
		}
	}
	return writeErr
}

// Merge converts and merges an event iterator into the managed UI stream.
func (sw *UIStreamWriter) Merge(events iter.Seq2[aikit.StepEvent, error], msgID string, opts ToUIStreamOptions) error {
	chunks := ToUIMessageStream(events, msgID, opts)
	return sw.mergeChunks(chunks)
}

// CreateUIMessageStream creates a managed UI message stream.
// It emits start, runs the execute callback, then emits finish + [DONE].
// The execute callback receives a UIStreamWriter for writing custom data and merging model streams.
func CreateUIMessageStream(w io.Writer, opts CreateUIStreamOptions, execute func(sw *UIStreamWriter) error) {
	// Emit start.
	startFields := map[string]any{"messageId": opts.MessageID}
	if opts.Metadata != nil {
		startFields["messageMetadata"] = opts.Metadata
	}
	if err := WriteSSE(w, Chunk{Type: ChunkStart, Fields: startFields}); err != nil {
		// Client disconnected before the stream began.
		if opts.OnEnd != nil {
			opts.OnEnd(UIStreamEndResult{FinishReason: "error"})
		}
		return
	}

	// Create stream writer.
	sw := &UIStreamWriter{
		writer: NewWriter(w),
	}

	// Run execute.
	err := execute(sw)

	// Determine finish reason.
	finishReason := "stop"
	if sw.lastFinish != "" {
		finishReason = sw.lastFinish
	}
	// A failed terminal write means the client disconnected; the flag reflects it
	// so OnEnd (which may drive server-side persistence) sees an error finish.
	clientGone := false
	if err != nil {
		// Redact by default; OnError is the consumer's own code and receives the
		// full typed error, so it can choose what (if anything) to surface.
		errMsg := redactStreamError(err)
		if opts.OnError != nil {
			if custom := opts.OnError(err); custom != "" {
				errMsg = custom
			}
		}
		finishReason = "error"
		if werr := sw.writer.writeTrustedError(errMsg); werr != nil {
			clientGone = true
		}
	}

	if werr := sw.writer.WriteFinishWithReason(finishReason, nil); werr != nil {
		clientGone = true
	}
	if clientGone {
		finishReason = "error"
	}

	if opts.OnEnd != nil {
		opts.OnEnd(UIStreamEndResult{
			Text:         sw.text,
			FinishReason: finishReason,
		})
	}
}
