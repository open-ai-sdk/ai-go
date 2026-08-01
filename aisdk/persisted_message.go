package aisdk

import (
	"encoding/json"
	"strings"
)

// PersistedMessageBuilder accumulates typed parts from stream chunks
// for persistence to a database.
type PersistedMessageBuilder struct {
	textAccum      strings.Builder
	reasoningAccum strings.Builder
	lastSignature  string

	pendingTool map[string]*toolInvocationPart
	parts       []any
	metadata    json.RawMessage
}

type toolInvocationPart struct {
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
	State      string `json:"state"`
	Input      any    `json:"input,omitempty"`
	Output     any    `json:"output,omitempty"`
	ErrorText  string `json:"errorText,omitempty"`
}

func NewPersistedMessageBuilder() *PersistedMessageBuilder {
	return &PersistedMessageBuilder{
		pendingTool: make(map[string]*toolInvocationPart),
	}
}

func (b *PersistedMessageBuilder) ObserveChunk(c Chunk) {
	switch c.Type {
	case ChunkTextStart:
		b.observeTextStart()
	case ChunkTextDelta:
		b.observeTextDelta(c)
	case ChunkTextEnd:
		b.observeTextEnd()
	case ChunkReasoningStart:
		b.observeReasoningStart()
	case ChunkReasoningDelta:
		b.observeReasoningDelta(c)
	case ChunkReasoningEnd:
		b.observeReasoningEnd(c)
	case ChunkToolInputAvailable:
		b.observeToolInput(c)
	case ChunkToolOutputAvailable:
		b.observeToolOutput(c)
	case ChunkToolInputError:
		b.observeToolInputError(c)
	case ChunkToolOutputError:
		b.observeToolOutputError(c)
	case ChunkToolOutputDenied:
		b.observeToolOutputDenied(c)
	case ChunkSourceURL:
		b.observeSourceURL(c)
	case ChunkSourceDocument:
		b.observeSourceDocument(c)
	case ChunkStartStep:
		b.observeStartStep()
	case ChunkFile:
		b.observeFile(c)
	case ChunkMessageMetadata:
		b.observeMessageMetadata(c)
	default:
		b.observeDataChunk(c)
	}
}

func (b *PersistedMessageBuilder) observeStartStep() {
	b.parts = append(b.parts, map[string]any{"type": "step-start"})
}

func (b *PersistedMessageBuilder) observeTextStart() {
	b.textAccum.Reset()
}

func (b *PersistedMessageBuilder) observeTextDelta(c Chunk) {
	if delta, ok := c.Fields["delta"].(string); ok {
		b.textAccum.WriteString(delta)
	}
}

func (b *PersistedMessageBuilder) observeTextEnd() {
	text := b.textAccum.String()
	if text != "" {
		b.parts = append(b.parts, map[string]any{"type": "text", "text": text})
		b.textAccum.Reset()
	}
}

func (b *PersistedMessageBuilder) observeReasoningStart() {
	b.reasoningAccum.Reset()
	b.lastSignature = ""
}

func (b *PersistedMessageBuilder) observeReasoningDelta(c Chunk) {
	if delta, ok := c.Fields["delta"].(string); ok {
		b.reasoningAccum.WriteString(delta)
	}
	if sig, ok := c.Fields["signature"].(string); ok && sig != "" {
		b.lastSignature = sig
	}
}

func (b *PersistedMessageBuilder) observeMessageMetadata(c Chunk) {
	if meta := c.Fields["messageMetadata"]; meta != nil {
		if raw, err := json.Marshal(meta); err == nil {
			b.metadata = raw
		}
	}
}

func (b *PersistedMessageBuilder) observeReasoningEnd(c Chunk) {
	if sig, ok := c.Fields["signature"].(string); ok && sig != "" {
		b.lastSignature = sig
	}
	reasoning := b.reasoningAccum.String()
	if reasoning != "" {
		part := map[string]any{"type": "reasoning", "reasoning": reasoning}
		if b.lastSignature != "" {
			part["signature"] = b.lastSignature
		}
		b.parts = append(b.parts, part)
		b.reasoningAccum.Reset()
		b.lastSignature = ""
	}
}

func (b *PersistedMessageBuilder) observeSourceURL(c Chunk) {
	id, ok1 := c.Fields["sourceId"].(string)
	url, ok2 := c.Fields["url"].(string)
	title, ok3 := c.Fields["title"].(string)
	_ = ok1
	_ = ok2
	_ = ok3
	b.parts = append(b.parts, map[string]any{
		"type": "source-url", "id": id, "url": url, "title": title,
	})
}

func (b *PersistedMessageBuilder) observeSourceDocument(c Chunk) {
	id, ok1 := c.Fields["sourceId"].(string)
	title, ok2 := c.Fields["title"].(string)
	mediaType, ok3 := c.Fields["mediaType"].(string)
	_ = ok1
	_ = ok2
	_ = ok3
	part := map[string]any{
		"type": "source-document", "id": id, "title": title, "mediaType": mediaType,
	}
	if fn, ok := c.Fields["filename"].(string); ok && fn != "" {
		part["filename"] = fn
	}
	if data := c.Fields["data"]; data != nil {
		part["data"] = data
	}
	if pm := c.Fields["providerMetadata"]; pm != nil {
		part["providerMetadata"] = pm
	}
	b.parts = append(b.parts, part)
}

func (b *PersistedMessageBuilder) observeFile(c Chunk) {
	part := map[string]any{"type": "file"}
	for _, key := range []string{"url", "mediaType", "name", "id", "fileId"} {
		if v, ok := c.Fields[key].(string); ok && v != "" {
			part[key] = v
		}
	}
	if data := c.Fields["data"]; data != nil {
		part["data"] = data
	}
	if pm := c.Fields["providerMetadata"]; pm != nil {
		part["providerMetadata"] = pm
	}
	b.parts = append(b.parts, part)
}

// copyOptionalBool copies a bool field from src to dst if present.
func copyOptionalBool(dst, src map[string]any, key string) {
	if v, ok := src[key].(bool); ok {
		dst[key] = v
	}
}

// copyOptionalString copies a non-empty string field from src to dst if present.
func copyOptionalString(dst, src map[string]any, key string) {
	if v, ok := src[key].(string); ok && v != "" {
		dst[key] = v
	}
}

// applyV6ToolFields copies optional v6 bool/string fields from a chunk into a part map.
func applyV6ToolFields(part, fields map[string]any) {
	copyOptionalBool(part, fields, "providerExecuted")
	copyOptionalBool(part, fields, "dynamic")
	copyOptionalBool(part, fields, "preliminary")
	copyOptionalString(part, fields, "title")
}

func (b *PersistedMessageBuilder) observeDataChunk(c Chunk) {
	// data-* chunks (non-transient); transient-data-* are excluded from parts.
	if !strings.HasPrefix(c.Type, "data-") || strings.HasPrefix(c.Type, "transient-data-") {
		return
	}
	transient, ok := c.Fields["transient"].(bool)
	_ = ok
	if transient {
		return
	}
	name := strings.TrimPrefix(c.Type, "data-")
	b.parts = append(b.parts, map[string]any{
		"type": "data", "name": name, "data": c.Fields["data"], "isTransient": false,
	})
}
