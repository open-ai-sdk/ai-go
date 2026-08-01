package aisdk

import (
	"encoding/json"
)

// UIMessagePart represents a typed part accumulated from stream chunks.
type UIMessagePart = map[string]any

// StreamingUIMessage represents the assistant message being built from stream chunks.
type StreamingUIMessage struct {
	ID       string          `json:"id"`
	Role     string          `json:"role"`
	Parts    []UIMessagePart `json:"parts"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// StreamingUIMessageState holds the mutable state used during stream processing.
// It tracks the in-flight text, reasoning, and tool-call parts.
type StreamingUIMessageState struct {
	Message              StreamingUIMessage
	ActiveTextParts      map[string]*UIMessagePart // keyed by chunk id
	ActiveReasoningParts map[string]*UIMessagePart
	PartialToolCalls     map[string]*partialToolCall
	FinishReason         string
}

type partialToolCall struct {
	Text     string
	Index    int
	ToolName string
}

// NewStreamingUIMessageState creates a new state, optionally continuing from a
// previous assistant message.
func NewStreamingUIMessageState(messageID string, lastMessage *StreamingUIMessage) *StreamingUIMessageState {
	var msg StreamingUIMessage
	if lastMessage != nil && lastMessage.Role == "assistant" {
		// Clone to avoid mutating the caller's message.
		msg = *lastMessage
		parts := make([]UIMessagePart, len(lastMessage.Parts))
		copy(parts, lastMessage.Parts)
		msg.Parts = parts
	} else {
		msg = StreamingUIMessage{
			ID:   messageID,
			Role: "assistant",
		}
	}

	return &StreamingUIMessageState{
		Message:              msg,
		ActiveTextParts:      make(map[string]*UIMessagePart),
		ActiveReasoningParts: make(map[string]*UIMessagePart),
		PartialToolCalls:     make(map[string]*partialToolCall),
	}
}

// ProcessUIMessageStream consumes a channel of Chunks, updates state on each chunk
// to reconstruct the assistant message, and re-emits every chunk on the returned channel.
//
// Server code produces streams and does not consume its own browser protocol.
func ProcessUIMessageStream(chunks <-chan Chunk, state *StreamingUIMessageState) <-chan Chunk {
	out := make(chan Chunk, 64)
	go func() {
		defer close(out)
		defer recoverPanic(recoverToTerminalChunks(out))
		for c := range chunks {
			processChunkIntoState(c, state)
			out <- c
		}
	}()
	return out
}

type chunkStateHandler func(c Chunk, state *StreamingUIMessageState)

var chunkStateHandlers = map[string]chunkStateHandler{
	ChunkStart:               handleChunkStart,
	ChunkStartStep:           handleChunkStartStep,
	ChunkFinishStep:          handleChunkFinishStep,
	ChunkTextStart:           handleChunkTextStart,
	ChunkTextDelta:           handleChunkTextDelta,
	ChunkTextEnd:             handleChunkTextEnd,
	ChunkReasoningStart:      handleChunkReasoningStart,
	ChunkReasoningDelta:      handleChunkReasoningDelta,
	ChunkReasoningEnd:        handleChunkReasoningEnd,
	ChunkToolInputStart:      handleChunkToolInputStart,
	ChunkToolInputDelta:      handleChunkToolInputDelta,
	ChunkToolInputAvailable:  handleChunkToolInputAvailable,
	ChunkToolOutputAvailable: handleChunkToolOutputAvailable,
	ChunkToolInputError:      handleChunkToolInputError,
	ChunkToolOutputError:     handleChunkToolOutputError,
	ChunkToolOutputDenied:    handleChunkToolOutputDenied,
	ChunkSourceURL:           handleChunkSourceURL,
	ChunkSourceDocument:      handleChunkSourceDocument,
	ChunkFile:                handleChunkFile,
	ChunkFinish:              handleChunkFinish,
	ChunkMessageMetadata:     handleChunkMessageMetadata,
	ChunkError:               handleChunkError,
}

// processChunkIntoState mutates state based on the incoming chunk.
func processChunkIntoState(c Chunk, state *StreamingUIMessageState) {
	if handler := chunkStateHandlers[c.Type]; handler != nil {
		handler(c, state)
	}
}

func handleChunkStart(c Chunk, state *StreamingUIMessageState) {
	if id, ok := c.Fields["messageId"].(string); ok && id != "" {
		state.Message.ID = id
	}
	mergeMessageMetadata(state, c.Fields)
}

func handleChunkStartStep(_ Chunk, state *StreamingUIMessageState) {
	state.Message.Parts = append(state.Message.Parts, UIMessagePart{"type": "step-start"})
}

func handleChunkFinishStep(_ Chunk, state *StreamingUIMessageState) {
	state.ActiveTextParts = make(map[string]*UIMessagePart)
	state.ActiveReasoningParts = make(map[string]*UIMessagePart)
}

func handleChunkSourceURL(c Chunk, state *StreamingUIMessageState) {
	state.Message.Parts = append(state.Message.Parts, UIMessagePart{
		"type":     "source-url",
		"sourceId": c.Fields["sourceId"],
		"url":      c.Fields["url"],
		"title":    c.Fields["title"],
	})
}

func handleChunkSourceDocument(c Chunk, state *StreamingUIMessageState) {
	part := UIMessagePart{
		"type":      "source-document",
		"sourceId":  c.Fields["sourceId"],
		"mediaType": c.Fields["mediaType"],
		"title":     c.Fields["title"],
	}
	if fn, ok := c.Fields["filename"].(string); ok && fn != "" {
		part["filename"] = fn
	}
	state.Message.Parts = append(state.Message.Parts, part)
}

func handleChunkFile(c Chunk, state *StreamingUIMessageState) {
	part := UIMessagePart{"type": "file"}
	for _, key := range []string{"url", "mediaType", "name"} {
		if v, ok := c.Fields[key].(string); ok && v != "" {
			part[key] = v
		}
	}
	state.Message.Parts = append(state.Message.Parts, part)
}

func handleChunkFinish(c Chunk, state *StreamingUIMessageState) {
	if fr, ok := c.Fields["finishReason"].(string); ok {
		state.FinishReason = fr
	}
	mergeMessageMetadata(state, c.Fields)
}

func handleChunkMessageMetadata(c Chunk, state *StreamingUIMessageState) {
	mergeMessageMetadata(state, c.Fields)
}

func handleChunkError(_ Chunk, _ *StreamingUIMessageState) {
	// Errors are passed through but don't mutate message state.
}

// chunkID extracts the "id" field from a chunk.
func chunkID(c Chunk) string {
	return stringField(c.Fields, "id")
}

func stringField(fields map[string]any, key string) string {
	value, ok := fields[key].(string)
	if !ok {
		return ""
	}
	return value
}

// mergeMessageMetadata merges messageMetadata from chunk fields into the state.
func mergeMessageMetadata(state *StreamingUIMessageState, fields map[string]any) {
	if md := fields["messageMetadata"]; md != nil {
		if raw, err := json.Marshal(md); err == nil {
			state.Message.Metadata = raw
		}
	}
}
