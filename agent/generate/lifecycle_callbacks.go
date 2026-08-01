package generate

import "github.com/open-ai-sdk/ai-go/agent"

func lifecycleCallbacks(req GenerateTextRequest) *agent.LifecycleCallbacks {
	if req.OnStepEnd == nil && req.OnEnd == nil && req.OnChunk == nil && req.OnError == nil {
		return nil
	}

	callbacks := &agent.LifecycleCallbacks{OnError: req.OnError}
	var current StepOutput
	var capturedSteps []StepOutput
	captureResults := req.OnStepEnd != nil || req.OnEnd != nil
	if captureResults {
		callbacks.OnStepEnd = func(event agent.StepEndEvent) {
			current.Text = event.Text
			current.Reasoning = event.Reasoning
			current.ToolCalls = event.ToolCalls
			current.ToolResults = event.ToolResults
			current.FinishReason = event.FinishReason
			current.ProviderMetadata = event.ProviderMetadata
			current.Warnings = event.Warnings
			if event.Usage != nil {
				current.Usage = *event.Usage
			}
			current.Response = Response{Messages: ResponseMessagesForStep(current, req.Tools)}
			completed := snapshotStepOutput(current)
			capturedSteps = append(capturedSteps, completed)
			if req.OnStepEnd == nil {
				return
			}
			publicEvent := StepEndEvent{
				StepNumber:       event.StepNumber,
				Text:             event.Text,
				Reasoning:        event.Reasoning,
				Content:          cloneContentParts(completed.Content),
				Files:            cloneGeneratedFiles(completed.Files),
				ToolCalls:        event.ToolCalls,
				ToolResults:      event.ToolResults,
				FinishReason:     event.FinishReason,
				Usage:            event.Usage,
				ProviderMetadata: event.ProviderMetadata,
				Warnings:         event.Warnings,
			}
			publicEvent.Response = snapshotResponse(completed.Response)
			req.OnStepEnd(publicEvent)
		}
	}
	if req.OnEnd != nil {
		callbacks.OnEnd = func(event agent.EndEvent) {
			steps := make([]StepOutput, len(event.Steps))
			for i, step := range event.Steps {
				steps[i] = StepOutput{
					Text:             step.Text,
					Reasoning:        step.Reasoning,
					ToolCalls:        step.ToolCalls,
					ToolResults:      step.ToolResults,
					FinishReason:     step.FinishReason,
					RawFinishReason:  step.RawFinishReason,
					ProviderMetadata: step.ProviderMetadata,
					Warnings:         step.Warnings,
				}
				if step.Usage != nil {
					steps[i].Usage = *step.Usage
				}
				if i < len(capturedSteps) {
					steps[i].Content = cloneContentParts(capturedSteps[i].Content)
					steps[i].Files = cloneGeneratedFiles(capturedSteps[i].Files)
				}
				steps[i].Response = Response{Messages: ResponseMessagesForStep(steps[i], req.Tools)}
			}
			req.OnEnd(EndEvent{
				Text:             event.Text,
				Reasoning:        event.Reasoning,
				Steps:            steps,
				Usage:            event.Usage,
				FinishReason:     event.FinishReason,
				ProviderMetadata: event.ProviderMetadata,
				Response:         Response{Messages: ResponseMessagesForSteps(steps, req.Tools)},
			})
		}
	}
	if captureResults || req.OnChunk != nil {
		callbacks.OnChunk = func(event StepEvent) {
			if captureResults {
				captureLifecycleContent(&current, event)
			}
			if req.OnChunk != nil {
				req.OnChunk(chunkEvent(event))
			}
		}
	}
	return callbacks
}

func captureLifecycleContent(current *StepOutput, event StepEvent) {
	switch event.Type {
	case StepEventStepStart:
		*current = StepOutput{}
	case StepEventTextDelta:
		current.Text += event.TextDelta
		appendStepText(current, event.TextDelta, event.ThoughtSignature)
	case StepEventReasoningDelta:
		current.Reasoning += event.ReasoningDelta
		appendStepReasoning(current, event.ReasoningDelta, event.ThoughtSignature)
	case StepEventToolCallStart:
		handleToolCallStart(event, current)
	case StepEventToolCallDelta:
		handleToolCallDelta(event, current)
	case StepEventToolCallReady:
		handleToolCallReady(event, current)
	case StepEventToolApprovalRequest:
		handleToolApprovalRequest(event, current)
	case StepEventFileDelta:
		if len(event.FileData) == 0 {
			return
		}
		file := GeneratedFile{Data: append([]byte(nil), event.FileData...), MediaType: event.FileMediaType}
		current.Files = append(current.Files, file)
		current.Content = append(current.Content, ContentPart{
			Type: ContentPartTypeFile, Data: append([]byte(nil), event.FileData...),
			MediaType: event.FileMediaType,
		})
	}
}
