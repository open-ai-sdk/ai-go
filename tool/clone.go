package tool

import (
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/internal/jsonclone"
)

func cloneDefinition(definition aikit.ToolDefinition) aikit.ToolDefinition {
	definition.InputSchema = cloneJSONMap(definition.InputSchema)
	definition.ContextSchema = cloneJSONMap(definition.ContextSchema)
	return definition
}

func cloneJSONMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned, ok := cloneJSONValue(input).(map[string]any)
	if !ok {
		panic("tool: cloned JSON map changed type")
	}
	return cloned
}

func cloneJSONValue(input any) any {
	return jsonclone.ValueWithPointers(input)
}
