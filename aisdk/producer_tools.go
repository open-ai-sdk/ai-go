package aisdk

import (
	"encoding/json"
	"fmt"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func (cp *ChunkProducer) chunksToolCallStart(event aikit.StepEvent) []Chunk {
	id := event.ToolCallID
	if id == "" {
		return nil
	}
	cp.toolInputStarted[id] = true
	cp.toolArgsAccum[id] = event.ToolCallArgsDelta
	chunks := cp.chunksBlockEnd()
	if len(chunks) > 0 {
		cp.advanceBlockID()
	}
	chunks = append(chunks, Chunk{Type: ChunkToolInputStart, Fields: map[string]any{
		"toolCallId": id, "toolName": event.ToolCallName,
	}})
	if event.ToolCallArgsDelta != "" {
		chunks = append(chunks, Chunk{Type: ChunkToolInputDelta, Fields: map[string]any{
			"toolCallId": id, "inputTextDelta": event.ToolCallArgsDelta,
		}})
	}
	return chunks
}

func (cp *ChunkProducer) chunksToolCallDelta(event aikit.StepEvent) []Chunk {
	id := event.ToolCallID
	if !cp.toolInputStarted[id] || event.ToolCallArgsDelta == "" || json.Valid([]byte(cp.toolArgsAccum[id])) {
		return nil
	}
	cp.toolArgsAccum[id] += event.ToolCallArgsDelta
	return []Chunk{{Type: ChunkToolInputDelta, Fields: map[string]any{
		"toolCallId": id, "inputTextDelta": event.ToolCallArgsDelta,
	}}}
}

func (cp *ChunkProducer) chunksToolCallReady(event aikit.StepEvent) []Chunk {
	id := event.ToolCallID
	if id == "" || cp.toolInputReady[id] {
		return nil
	}
	args := event.ToolCallArgsDelta
	if args == "" {
		args = cp.toolArgsAccum[id]
	}
	cp.toolInputReady[id] = true
	return []Chunk{{Type: ChunkToolInputAvailable, Fields: withProviderMetadata(map[string]any{
		"toolCallId": id, "toolName": event.ToolCallName, "input": parseToolArgs(args),
	}, event.ProviderMetadata)}}
}

func (cp *ChunkProducer) chunksToolResult(event aikit.StepEvent) []Chunk {
	if event.ToolResult == nil {
		return nil
	}
	result := event.ToolResult
	chunks := make([]Chunk, 0, 2)
	if result.ID != "" && !cp.toolInputReady[result.ID] {
		cp.toolInputReady[result.ID] = true
		chunks = append(chunks, Chunk{Type: ChunkToolInputAvailable, Fields: withProviderMetadata(map[string]any{
			"toolCallId": result.ID, "toolName": result.Name, "input": parseToolArgs(result.Args),
		}, event.ProviderMetadata)})
	}
	return append(chunks, Chunk{Type: ChunkToolOutputAvailable, Fields: withProviderMetadata(map[string]any{
		"toolCallId": result.ID, "output": result.Output,
	}, event.ProviderMetadata)})
}

func parseToolArgs(args string) any {
	var parsed any
	if err := json.Unmarshal([]byte(args), &parsed); err != nil {
		return map[string]string{"raw": args}
	}
	return parsed
}

func (cp *ChunkProducer) chunksToolCallInvalid(event aikit.StepEvent) []Chunk {
	return []Chunk{{Type: ChunkToolInputError, Fields: map[string]any{
		"toolCallId": event.ToolCallID, "toolName": event.ToolCallName,
		"errorText": fmt.Sprintf("invalid JSON arguments for tool %q", event.ToolCallName),
	}}}
}

func (cp *ChunkProducer) chunksSource(event aikit.StepEvent) []Chunk {
	if event.Source == nil || event.Source.URL == "" {
		return nil
	}
	return []Chunk{{Type: ChunkSourceURL, Fields: withProviderMetadata(map[string]any{
		"sourceId": event.Source.ID, "url": event.Source.URL, "title": event.Source.Title,
	}, event.Source.ProviderMetadata)}}
}
