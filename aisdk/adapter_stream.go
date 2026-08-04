package aisdk

import (
	"io"
	"iter"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func (adapter *Adapter) Stream(events iter.Seq2[aikit.StepEvent, error], output io.Writer) string {
	state := &interceptState{}
	if adapter.toolResultHook != nil {
		state.toolCache = make(map[string]toolData)
	}
	stream := NewChunkProducer(adapter.msgID).Produce(adapter.interceptEvents(eventChannel(events), state))
	writer := NewWriter(output)
	var finishReason string
	var writeErr error
	for chunk := range stream.Chunks {
		if adapter.persistenceBuilder != nil {
			adapter.persistenceBuilder.ObserveChunk(chunk)
		}
		if writeErr != nil {
			continue
		}
		switch chunk.Type {
		case ChunkFinish:
			if reason, ok := chunk.Fields["finishReason"].(string); ok {
				finishReason = reason
			}
			state.mu.Lock()
			usage := state.usage.snapshot()
			state.mu.Unlock()
			writeErr = writer.WriteFinishWithReason(finishReason, usageMetadata(usage))
		case ChunkError:
			writeErr = writer.writeChunk(chunk)
		default:
			writeErr = adapter.writeChunkWithHooks(writer, chunk, state)
		}
	}
	text := stream.FullText()
	if adapter.onEnd != nil {
		adapter.onEnd(text, finishReason)
	}
	return text
}
