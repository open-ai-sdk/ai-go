package aisdk

func (b *PersistedMessageBuilder) observeToolInput(chunk Chunk) {
	id := stringField(chunk.Fields, "toolCallId")
	if id == "" {
		return
	}
	tool := b.pendingTool[id]
	if tool == nil {
		tool = &toolInvocationPart{ToolCallID: id, State: "input-available"}
		b.pendingTool[id] = tool
	}
	tool.Input = chunk.Fields["input"]
	tool.ToolName = stringField(chunk.Fields, "toolName")
}

func (b *PersistedMessageBuilder) observeToolOutput(chunk Chunk) {
	id := stringField(chunk.Fields, "toolCallId")
	if id == "" {
		return
	}
	tool := b.pendingTool[id]
	if tool == nil {
		tool = &toolInvocationPart{ToolCallID: id}
	}
	tool.Output, tool.State = chunk.Fields["output"], "output-available"
	part := map[string]any{
		"type": "tool-invocation", "toolCallId": id, "toolName": tool.ToolName,
		"state": tool.State, "input": tool.Input, "output": tool.Output,
	}
	applyV6ToolFields(part, chunk.Fields)
	b.parts = append(b.parts, part)
	delete(b.pendingTool, id)
}

func (b *PersistedMessageBuilder) observeToolInputError(chunk Chunk) {
	id := stringField(chunk.Fields, "toolCallId")
	if id == "" {
		return
	}
	part := map[string]any{
		"type": "tool-invocation", "toolCallId": id,
		"toolName": stringField(chunk.Fields, "toolName"), "state": "error",
		"input": chunk.Fields["input"], "errorText": chunk.Fields["errorText"],
	}
	applyV6ToolFields(part, chunk.Fields)
	b.parts = append(b.parts, part)
	delete(b.pendingTool, id)
}

func (b *PersistedMessageBuilder) observeToolOutputError(chunk Chunk) {
	b.observeToolTerminal(chunk, "error", true)
}

func (b *PersistedMessageBuilder) observeToolOutputDenied(chunk Chunk) {
	b.observeToolTerminal(chunk, "denied", false)
}

func (b *PersistedMessageBuilder) observeToolTerminal(chunk Chunk, state string, includeError bool) {
	id := stringField(chunk.Fields, "toolCallId")
	if id == "" {
		return
	}
	tool := b.pendingTool[id]
	if tool == nil {
		tool = &toolInvocationPart{ToolCallID: id}
	}
	part := map[string]any{
		"type": "tool-invocation", "toolCallId": id, "toolName": tool.ToolName,
		"state": state, "input": tool.Input,
	}
	if includeError {
		part["errorText"] = chunk.Fields["errorText"]
	}
	applyV6ToolFields(part, chunk.Fields)
	b.parts = append(b.parts, part)
	delete(b.pendingTool, id)
}
