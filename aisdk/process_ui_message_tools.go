package aisdk

import "encoding/json"

func handleChunkToolInputStart(chunk Chunk, state *StreamingUIMessageState) {
	id := stringField(chunk.Fields, "toolCallId")
	if id == "" {
		return
	}
	name := stringField(chunk.Fields, "toolName")
	state.PartialToolCalls[id] = &partialToolCall{ToolName: name, Index: len(state.Message.Parts)}
	state.Message.Parts = append(state.Message.Parts, UIMessagePart{
		"type": "tool-invocation", "toolCallId": id, "toolName": name, "state": "input-streaming",
	})
}

func handleChunkToolInputDelta(chunk Chunk, state *StreamingUIMessageState) {
	id := stringField(chunk.Fields, "toolCallId")
	partial := state.PartialToolCalls[id]
	if partial == nil {
		return
	}
	delta, ok := chunk.Fields["inputTextDelta"].(string)
	if !ok {
		return
	}
	partial.Text += delta
	if partial.Index < len(state.Message.Parts) && json.Valid([]byte(partial.Text)) {
		var input any
		if json.Unmarshal([]byte(partial.Text), &input) == nil {
			state.Message.Parts[partial.Index]["input"] = input
		}
	}
}

func handleChunkToolInputAvailable(chunk Chunk, state *StreamingUIMessageState) {
	id, name := stringField(chunk.Fields, "toolCallId"), stringField(chunk.Fields, "toolName")
	if id == "" {
		return
	}
	updateOrAddToolPart(state, id, UIMessagePart{
		"type": "tool-invocation", "toolCallId": id, "toolName": name,
		"state": "input-available", "input": chunk.Fields["input"],
	})
}

func handleChunkToolInputError(chunk Chunk, state *StreamingUIMessageState) {
	id := stringField(chunk.Fields, "toolCallId")
	updateOrAddToolPart(state, id, UIMessagePart{
		"type": "tool-invocation", "toolCallId": id,
		"toolName": stringField(chunk.Fields, "toolName"), "state": "output-error",
		"input": chunk.Fields["input"], "errorText": chunk.Fields["errorText"],
	})
}

func handleChunkToolOutputAvailable(chunk Chunk, state *StreamingUIMessageState) {
	updateToolPartFields(state, stringField(chunk.Fields, "toolCallId"), map[string]any{
		"state": "output-available", "output": chunk.Fields["output"],
	})
}

func handleChunkToolOutputError(chunk Chunk, state *StreamingUIMessageState) {
	updateToolPartFields(state, stringField(chunk.Fields, "toolCallId"), map[string]any{
		"state": "output-error", "errorText": chunk.Fields["errorText"],
	})
}

func handleChunkToolOutputDenied(chunk Chunk, state *StreamingUIMessageState) {
	updateToolPartFields(state, stringField(chunk.Fields, "toolCallId"), map[string]any{"state": "output-denied"})
}

func updateOrAddToolPart(state *StreamingUIMessageState, id string, part UIMessagePart) {
	if id == "" {
		return
	}
	for index, existing := range state.Message.Parts {
		if existing["toolCallId"] == id &&
			(existing["type"] == "tool-invocation" || existing["type"] == "dynamic-tool") {
			for key, value := range part {
				state.Message.Parts[index][key] = value
			}
			return
		}
	}
	state.Message.Parts = append(state.Message.Parts, part)
}

func updateToolPartFields(state *StreamingUIMessageState, id string, fields map[string]any) {
	if id == "" {
		return
	}
	for index, part := range state.Message.Parts {
		if part["toolCallId"] == id && (part["type"] == "tool-invocation" || part["type"] == "dynamic-tool") {
			for key, value := range fields {
				state.Message.Parts[index][key] = value
			}
			return
		}
	}
}
