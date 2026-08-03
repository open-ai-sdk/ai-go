package generate

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

func snapshotStepEvent(event StepEvent) StepEvent {
	snapshot := event
	snapshot.Usage = snapshotUsage(event.Usage)
	snapshot.ProviderMetadata = snapshotJSONMap(event.ProviderMetadata)
	snapshot.Warnings = append([]Warning(nil), event.Warnings...)
	snapshot.FileData = append([]byte(nil), event.FileData...)
	snapshot.StructuredOutput = append(json.RawMessage(nil), event.StructuredOutput...)
	if event.ToolResult != nil {
		result := event.ToolResult.Clone()
		snapshot.ToolResult = &result
	}
	if event.Source != nil {
		source := *event.Source
		source.ProviderMetadata = snapshotJSONMap(event.Source.ProviderMetadata)
		snapshot.Source = &source
	}
	return snapshot
}

func snapshotStepEndEvent(event StepEndEvent) StepEndEvent {
	snapshot := event
	snapshot.Content = cloneContentParts(event.Content)
	snapshot.Files = cloneGeneratedFiles(event.Files)
	snapshot.ToolCalls = snapshotToolCalls(event.ToolCalls)
	snapshot.ToolResults = snapshotToolResults(event.ToolResults)
	snapshot.Usage = snapshotUsage(event.Usage)
	snapshot.ProviderMetadata = snapshotJSONMap(event.ProviderMetadata)
	snapshot.Warnings = append([]Warning(nil), event.Warnings...)
	snapshot.Response = snapshotResponse(event.Response)
	return snapshot
}

func snapshotEndEvent(event EndEvent) EndEvent {
	snapshot := event
	if event.Steps != nil {
		snapshot.Steps = make([]StepOutput, len(event.Steps))
		for i, step := range event.Steps {
			snapshot.Steps[i] = snapshotStepOutput(step)
		}
	}
	snapshot.Usage = *snapshotUsage(&event.Usage)
	snapshot.ProviderMetadata = snapshotJSONMap(event.ProviderMetadata)
	snapshot.Response = snapshotResponse(event.Response)
	return snapshot
}

func snapshotChunkEvent(event ChunkEvent) ChunkEvent {
	snapshot := event
	snapshot.Usage = snapshotUsage(event.Usage)
	snapshot.ProviderMetadata = snapshotJSONMap(event.ProviderMetadata)
	snapshot.FileData = append([]byte(nil), event.FileData...)
	if event.ToolResult != nil {
		result := snapshotToolResults([]ToolResult{*event.ToolResult})[0]
		snapshot.ToolResult = &result
	}
	if event.Source != nil {
		source := *event.Source
		source.ProviderMetadata = snapshotJSONMap(event.Source.ProviderMetadata)
		snapshot.Source = &source
	}
	return snapshot
}

func snapshotStepOutput(step StepOutput) StepOutput {
	snapshot := step
	snapshot.Content = cloneContentParts(step.Content)
	snapshot.ToolCalls = snapshotToolCalls(step.ToolCalls)
	snapshot.ToolResults = snapshotToolResults(step.ToolResults)
	snapshot.Usage = *snapshotUsage(&step.Usage)
	snapshot.ProviderMetadata = snapshotJSONMap(step.ProviderMetadata)
	snapshot.Warnings = append([]Warning(nil), step.Warnings...)
	if step.Sources != nil {
		snapshot.Sources = make([]Source, len(step.Sources))
		for i, source := range step.Sources {
			snapshot.Sources[i] = source
			snapshot.Sources[i].ProviderMetadata = snapshotJSONMap(source.ProviderMetadata)
		}
	}
	if step.Files != nil {
		snapshot.Files = cloneGeneratedFiles(step.Files)
	}
	snapshot.Response = snapshotResponse(step.Response)
	return snapshot
}

func cloneGeneratedFiles(files []GeneratedFile) []GeneratedFile {
	if files == nil {
		return nil
	}
	cloned := make([]GeneratedFile, len(files))
	for i, file := range files {
		cloned[i] = file
		cloned[i].Data = append([]byte(nil), file.Data...)
	}
	return cloned
}

func snapshotToolCalls(calls []ToolCallOutput) []ToolCallOutput {
	if calls == nil {
		return nil
	}
	snapshot := make([]ToolCallOutput, len(calls))
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
		snapshot[i] = result.Clone()
	}
	return snapshot
}

func snapshotResponse(response Response) Response {
	snapshot := response
	snapshot.Messages = cloneMessages(response.Messages)
	return snapshot
}

func snapshotJSONMap(values map[string]any) map[string]any {
	return jsonclone.Map(values)
}
