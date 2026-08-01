package generate

import "github.com/open-ai-sdk/ai-go/agent"

func lifecycleCallbacks(req GenerateTextRequest) *agent.LifecycleCallbacks {
	if req.OnStepEnd == nil && req.OnEnd == nil && req.OnChunk == nil && req.OnError == nil {
		return nil
	}

	callbacks := &agent.LifecycleCallbacks{OnError: req.OnError}
	if req.OnStepEnd != nil {
		callbacks.OnStepEnd = func(event agent.StepEndEvent) {
			publicEvent := StepEndEvent{
				StepNumber:       event.StepNumber,
				Text:             event.Text,
				Reasoning:        event.Reasoning,
				ToolCalls:        event.ToolCalls,
				ToolResults:      event.ToolResults,
				FinishReason:     event.FinishReason,
				Usage:            event.Usage,
				ProviderMetadata: event.ProviderMetadata,
				Warnings:         event.Warnings,
			}
			publicEvent.Response = Response{Messages: ResponseMessagesForStep(StepOutput{
				Text:        publicEvent.Text,
				Reasoning:   publicEvent.Reasoning,
				ToolCalls:   publicEvent.ToolCalls,
				ToolResults: publicEvent.ToolResults,
			}, req.Tools)}
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
	if req.OnChunk != nil {
		callbacks.OnChunk = func(event StepEvent) {
			req.OnChunk(chunkEvent(event))
		}
	}
	return callbacks
}
