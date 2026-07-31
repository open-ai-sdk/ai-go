package engine

import (
	"encoding/json"

	"github.com/open-ai-sdk/ai-go/internal/jsonclone"
)

func snapshotUsage(usage *Usage) *Usage {
	if usage == nil {
		return nil
	}
	snapshot := *usage
	snapshot.Raw = snapshotJSONMap(usage.Raw)
	return &snapshot
}

func snapshotPrepareStepInfos(steps []StepResultInfo) []StepResultInfo {
	if steps == nil {
		return nil
	}
	snapshot := make([]StepResultInfo, len(steps))
	for i, step := range steps {
		snapshot[i] = step
		snapshot[i].ToolNames = append([]string(nil), step.ToolNames...)
		snapshot[i].ToolCalls = snapshotToolCallInfos(step.ToolCalls)
		snapshot[i].ToolResults = snapshotToolResults(step.ToolResults)
		snapshot[i].Usage = snapshotUsage(step.Usage)
		snapshot[i].ProviderMetadata = snapshotJSONMap(step.ProviderMetadata)
		snapshot[i].Warnings = append([]Warning(nil), step.Warnings...)
	}
	return snapshot
}

func snapshotToolCallInfos(calls []ToolCallInfo) []ToolCallInfo {
	if calls == nil {
		return nil
	}
	snapshot := make([]ToolCallInfo, len(calls))
	for i, call := range calls {
		snapshot[i] = call
		snapshot[i].Args = append(json.RawMessage(nil), call.Args...)
	}
	return snapshot
}

func snapshotToolResults(results []ToolResult) []ToolResult {
	if results == nil {
		return nil
	}
	snapshot := make([]ToolResult, len(results))
	for i, result := range results {
		snapshot[i] = snapshotToolResult(result)
	}
	return snapshot
}

func snapshotToolResult(result ToolResult) ToolResult {
	snapshot := result
	if result.Content != nil {
		snapshot.Content = make([]ToolResultContent, len(result.Content))
		for i, content := range result.Content {
			snapshot.Content[i] = content
			snapshot.Content[i].Data = append([]byte(nil), content.Data...)
		}
	}
	return snapshot
}

func snapshotMessages(messages []Message) []Message {
	if messages == nil {
		return nil
	}
	snapshot := make([]Message, len(messages))
	for i, message := range messages {
		snapshot[i] = message
		if message.Content == nil {
			continue
		}
		snapshot[i].Content = make([]ContentPart, len(message.Content))
		for j, content := range message.Content {
			snapshot[i].Content[j] = content
			snapshot[i].Content[j].Data = append([]byte(nil), content.Data...)
			snapshot[i].Content[j].ToolCallArgs = append(
				json.RawMessage(nil),
				content.ToolCallArgs...,
			)
		}
	}
	return snapshot
}

func snapshotToolSetForCallback(tools *ToolSet) *ToolSet {
	if tools == nil {
		return nil
	}
	definitions := make([]ToolDefinition, len(tools.Definitions))
	for i, definition := range tools.Definitions {
		definitions[i] = definition
		definitions[i].InputSchema = snapshotJSONMap(definition.InputSchema)
		definitions[i].ContextSchema = snapshotJSONMap(definition.ContextSchema)
	}
	return &ToolSet{
		Definitions: definitions,
		Executor:    tools.Executor,
	}
}

func snapshotStepEvent(event StepEvent) StepEvent {
	snapshot := event
	snapshot.Usage = snapshotUsage(event.Usage)
	snapshot.ProviderMetadata = snapshotJSONMap(event.ProviderMetadata)
	snapshot.Warnings = append([]Warning(nil), event.Warnings...)
	snapshot.FileData = append([]byte(nil), event.FileData...)
	snapshot.StructuredOutput = append(json.RawMessage(nil), event.StructuredOutput...)
	if event.ToolResult != nil {
		result := snapshotToolResult(*event.ToolResult)
		snapshot.ToolResult = &result
	}
	if event.Source != nil {
		source := *event.Source
		source.ProviderMetadata = snapshotJSONMap(event.Source.ProviderMetadata)
		snapshot.Source = &source
	}
	return snapshot
}

func snapshotJSONMap(values map[string]any) map[string]any {
	return jsonclone.Map(values)
}
