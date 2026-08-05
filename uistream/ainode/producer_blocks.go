package ainode

import "github.com/open-ai-sdk/ai-go/aikit"

func (cp *ChunkProducer) advanceBlockID() {
	cp.textBlockCount++
	cp.textBlockID = blockID(cp.textBlockCount)
}

func (cp *ChunkProducer) chunksStepStart() []Chunk {
	cp.advanceBlockID()
	cp.textStarted, cp.reasoningStarted = false, false
	cp.lastThoughtSignature = ""
	cp.toolInputStarted = make(map[string]bool)
	cp.toolArgsAccum = make(map[string]string)
	return []Chunk{{Type: ChunkStartStep}}
}

func (cp *ChunkProducer) chunksTextDelta(event aikit.StepEvent) ([]Chunk, string) {
	var chunks []Chunk
	if cp.reasoningStarted {
		fields := map[string]any{"id": cp.textBlockID}
		if cp.lastThoughtSignature != "" {
			fields["signature"] = cp.lastThoughtSignature
		}
		chunks = append(chunks, Chunk{Type: ChunkReasoningEnd, Fields: fields})
		cp.reasoningStarted = false
		cp.advanceBlockID()
	}
	if !cp.textStarted {
		chunks = append(chunks, Chunk{Type: ChunkTextStart, Fields: map[string]any{"id": cp.textBlockID}})
		cp.textStarted = true
	}
	fields := withProviderMetadata(map[string]any{
		"id": cp.textBlockID, "delta": event.TextDelta,
	}, event.ProviderMetadata)
	return append(chunks, Chunk{Type: ChunkTextDelta, Fields: fields}), event.TextDelta
}

func (cp *ChunkProducer) chunksReasoningDelta(event aikit.StepEvent) []Chunk {
	var chunks []Chunk
	if cp.textStarted {
		chunks = append(chunks, Chunk{Type: ChunkTextEnd, Fields: map[string]any{"id": cp.textBlockID}})
		cp.textStarted = false
		cp.advanceBlockID()
	}
	if !cp.reasoningStarted {
		chunks = append(chunks, Chunk{Type: ChunkReasoningStart, Fields: map[string]any{"id": cp.textBlockID}})
		cp.reasoningStarted = true
	}
	if event.ThoughtSignature != "" {
		cp.lastThoughtSignature = event.ThoughtSignature
	}
	fields := withProviderMetadata(map[string]any{
		"id": cp.textBlockID, "delta": event.ReasoningDelta,
	}, event.ProviderMetadata)
	return append(chunks, Chunk{Type: ChunkReasoningDelta, Fields: fields})
}

func (cp *ChunkProducer) chunksBlockEnd() []Chunk {
	var chunks []Chunk
	if cp.textStarted {
		chunks = append(chunks, Chunk{Type: ChunkTextEnd, Fields: map[string]any{"id": cp.textBlockID}})
		cp.textStarted = false
	}
	if cp.reasoningStarted {
		fields := map[string]any{"id": cp.textBlockID}
		if cp.lastThoughtSignature != "" {
			fields["signature"] = cp.lastThoughtSignature
		}
		chunks = append(chunks, Chunk{Type: ChunkReasoningEnd, Fields: fields})
		cp.reasoningStarted = false
	}
	return chunks
}

func (cp *ChunkProducer) chunksStepEnd() []Chunk {
	return append(cp.chunksBlockEnd(), Chunk{Type: ChunkFinishStep})
}

func (cp *ChunkProducer) chunksError(err error) []Chunk {
	chunks := append(cp.chunksBlockEnd(),
		Chunk{Type: ChunkError, Fields: map[string]any{"errorText": redactStreamError(err)}},
		Chunk{Type: ChunkFinish, Fields: map[string]any{"finishReason": "error"}},
	)
	cp.lastFinishReason = "error"
	return chunks
}
