package ainode

func handleChunkTextStart(chunk Chunk, state *StreamingUIMessageState) {
	id := chunkID(chunk)
	part := UIMessagePart{"type": "text", "text": ""}
	state.ActiveTextParts[id] = &part
	state.Message.Parts = append(state.Message.Parts, part)
}

func handleChunkTextDelta(chunk Chunk, state *StreamingUIMessageState) {
	appendChunkDeltaToPart(state.ActiveTextParts[chunkID(chunk)], chunk.Fields["delta"])
}

func handleChunkTextEnd(chunk Chunk, state *StreamingUIMessageState) {
	delete(state.ActiveTextParts, chunkID(chunk))
}

func handleChunkReasoningStart(chunk Chunk, state *StreamingUIMessageState) {
	id := chunkID(chunk)
	part := UIMessagePart{"type": "reasoning", "text": ""}
	state.ActiveReasoningParts[id] = &part
	state.Message.Parts = append(state.Message.Parts, part)
}

func handleChunkReasoningDelta(chunk Chunk, state *StreamingUIMessageState) {
	appendChunkDeltaToPart(state.ActiveReasoningParts[chunkID(chunk)], chunk.Fields["delta"])
}

func handleChunkReasoningEnd(chunk Chunk, state *StreamingUIMessageState) {
	delete(state.ActiveReasoningParts, chunkID(chunk))
}

func appendChunkDeltaToPart(part *UIMessagePart, raw any) {
	if part == nil {
		return
	}
	delta, ok := raw.(string)
	if !ok {
		return
	}
	existing, ok := (*part)["text"].(string)
	if !ok {
		existing = ""
	}
	(*part)["text"] = existing + delta
}
