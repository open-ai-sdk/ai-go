package ai

func chunkEvent(event StepEvent) ChunkEvent {
	var eventType string
	switch event.Type {
	case StepEventTextDelta:
		eventType = "text-delta"
	case StepEventReasoningDelta:
		eventType = "reasoning-delta"
	case StepEventToolCallStart:
		eventType = "tool-call-start"
	case StepEventToolCallDelta:
		eventType = "tool-call-delta"
	case StepEventToolCallReady:
		eventType = "tool-call-ready"
	case StepEventToolResult:
		eventType = "tool-result"
	case StepEventUsage:
		eventType = "usage"
	case StepEventStepStart:
		eventType = "step-start"
	case StepEventStepEnd:
		eventType = "step-end"
	case StepEventDone:
		eventType = "done"
	case StepEventError:
		eventType = "error"
	case StepEventSource:
		eventType = "source"
	case StepEventFileDelta:
		eventType = "file-delta"
	default:
		eventType = "unknown"
	}
	return ChunkEvent{
		Type:              eventType,
		TextDelta:         event.TextDelta,
		ReasoningDelta:    event.ReasoningDelta,
		ToolCallID:        event.ToolCallID,
		ToolCallName:      event.ToolCallName,
		ToolCallArgsDelta: event.ToolCallArgsDelta,
		StepNumber:        event.StepNumber,
		FinishReason:      event.FinishReason,
		Usage:             event.Usage,
		Source:            event.Source,
		ToolResult:        event.ToolResult,
		FileData:          event.FileData,
		FileMediaType:     event.FileMediaType,
		ProviderMetadata:  event.ProviderMetadata,
	}
}
