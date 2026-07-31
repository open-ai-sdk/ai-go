package ai

import (
	"context"
	"testing"
)

type requestRecordingModel struct {
	calls    [][]StreamEvent
	requests []LanguageModelRequest
}

func (m *requestRecordingModel) ModelID() string { return "request-recording" }

func (m *requestRecordingModel) Stream(
	_ context.Context,
	request LanguageModelRequest,
) (<-chan StreamEvent, error) {
	m.requests = append(m.requests, request)
	channel := make(chan StreamEvent, 8)
	events := m.calls[0]
	m.calls = m.calls[1:]
	for _, event := range events {
		channel <- event
	}
	close(channel)
	return channel, nil
}

type testToolExecutor struct{}

func (testToolExecutor) Execute(context.Context, string, string) (string, error) {
	return `{"ok":true}`, nil
}

type recordingToolExecutor struct {
	calls []string
}

func (e *recordingToolExecutor) Execute(
	_ context.Context,
	name string,
	_ string,
) (string, error) {
	e.calls = append(e.calls, name)
	return `{"ok":true}`, nil
}

type streamCountingModel struct {
	inner LanguageModel
	calls *int
}

func (m streamCountingModel) ModelID() string { return m.inner.ModelID() }

func (m streamCountingModel) Stream(
	ctx context.Context,
	request LanguageModelRequest,
) (<-chan StreamEvent, error) {
	*m.calls++
	return m.inner.Stream(ctx, request)
}

func countModelStreams(calls *int) LanguageModelMiddleware {
	return func(inner LanguageModel) LanguageModel {
		return streamCountingModel{inner: inner, calls: calls}
	}
}

func TestStreamResult_CumulativeUsageReplacesCurrentStepContribution(t *testing.T) {
	channel := make(chan StepEvent, 12)
	channel <- StepEvent{Type: StepEventStepStart, StepNumber: 0}
	channel <- StepEvent{Type: StepEventUsage, Usage: &Usage{
		InputTokens: 10, OutputTokens: 5, TotalTokens: 15,
	}}
	channel <- StepEvent{Type: StepEventUsage, Usage: &Usage{
		InputTokens: 10, OutputTokens: 20, TotalTokens: 30,
	}}
	channel <- StepEvent{Type: StepEventStepEnd, StepNumber: 0}
	channel <- StepEvent{Type: StepEventStepStart, StepNumber: 1}
	channel <- StepEvent{Type: StepEventUsage, Usage: &Usage{
		InputTokens: 3, OutputTokens: 2, TotalTokens: 5,
	}}
	channel <- StepEvent{Type: StepEventUsage, Usage: &Usage{
		InputTokens: 3, OutputTokens: 4, TotalTokens: 7,
	}}
	channel <- StepEvent{Type: StepEventStepEnd, StepNumber: 1}
	channel <- StepEvent{Type: StepEventDone}
	close(channel)

	result, err := NewStreamResult(channel).Consume()
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if got, want := result.Usage.InputTokens, 13; got != want {
		t.Fatalf("InputTokens = %d, want %d", got, want)
	}
	if got, want := result.Usage.OutputTokens, 24; got != want {
		t.Fatalf("OutputTokens = %d, want %d", got, want)
	}
	if got, want := result.Usage.TotalTokens, 37; got != want {
		t.Fatalf("TotalTokens = %d, want %d", got, want)
	}
	if got, want := result.FinalStep.Usage.TotalTokens, 7; got != want {
		t.Fatalf("FinalStep.Usage.TotalTokens = %d, want %d", got, want)
	}
}

func TestStreamResult_PartialErrorRetainsLatestUsageSnapshot(t *testing.T) {
	channel := make(chan StepEvent, 4)
	channel <- StepEvent{Type: StepEventStepStart}
	channel <- StepEvent{Type: StepEventUsage, Usage: &Usage{
		InputTokens: 10, OutputTokens: 5, TotalTokens: 15,
	}}
	channel <- StepEvent{Type: StepEventUsage, Usage: &Usage{
		InputTokens: 10, OutputTokens: 20, TotalTokens: 30,
	}}
	channel <- StepEvent{Type: StepEventError, Error: context.Canceled}
	close(channel)

	result, err := NewStreamResult(channel).Consume()
	if err == nil {
		t.Fatal("expected stream error")
	}
	if got, want := result.Usage.TotalTokens, 30; got != want {
		t.Fatalf("partial result TotalTokens = %d, want %d", got, want)
	}
}

func TestStreamResult_UsagePayloadIsIsolatedBetweenViews(t *testing.T) {
	source := make(chan StepEvent)
	result := NewStreamResult(source)
	first := result.Stream()
	second := result.Stream()

	source <- StepEvent{Type: StepEventUsage, Usage: &Usage{
		InputTokens: 1,
		Raw: map[string]any{
			"nested": map[string]any{"value": 1},
		},
	}}
	close(source)

	firstEvent := <-first
	secondEvent := <-second
	firstEvent.Usage.InputTokens = 999
	firstEvent.Usage.Raw["nested"].(map[string]any)["value"] = 999

	if got := secondEvent.Usage.InputTokens; got != 1 {
		t.Fatalf("second view InputTokens = %d, want 1", got)
	}
	if got := secondEvent.Usage.Raw["nested"].(map[string]any)["value"]; got != 1 {
		t.Fatalf("second view nested Raw value = %v, want 1", got)
	}
}

func TestActiveTools_ExplicitRestrictionFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		active []string
	}{
		{name: "empty", active: []string{}},
		{name: "unknown", active: []string{"missing"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := &requestRecordingModel{calls: [][]StreamEvent{{
				{Type: StreamEventFinish, FinishReason: FinishReasonStop},
			}}}
			_, err := GenerateText(context.Background(), GenerateTextRequest{
				Model: model,
				Tools: &ToolSet{Definitions: []ToolDefinition{{
					Name: "danger",
				}}},
				ActiveTools: test.active,
				MaxSteps:    1,
			})
			if err != nil {
				t.Fatalf("GenerateText: %v", err)
			}
			if got := len(model.requests[0].Tools); got != 0 {
				t.Fatalf("model received %d tools, want 0", got)
			}
		})
	}
}

func TestActiveTools_HiddenToolCannotExecute(t *testing.T) {
	for _, parallel := range []bool{false, true} {
		t.Run(map[bool]string{false: "sequential", true: "parallel"}[parallel], func(t *testing.T) {
			executor := &recordingToolExecutor{}
			model := &requestRecordingModel{calls: [][]StreamEvent{{
				{
					Type:              StreamEventToolCallDelta,
					ToolCallIndex:     0,
					ToolCallID:        "call-1",
					ToolCallName:      "danger",
					ToolCallArgsDelta: `{}`,
				},
				{Type: StreamEventFinish, FinishReason: FinishReasonToolCalls},
			}}}
			result, err := GenerateText(context.Background(), GenerateTextRequest{
				Model: model,
				Tools: &ToolSet{
					Definitions: []ToolDefinition{{Name: "danger"}},
					Executor:    executor,
				},
				ActiveTools:           []string{},
				ParallelToolExecution: parallel,
				MaxSteps:              1,
			})
			if err != nil {
				t.Fatalf("GenerateText: %v", err)
			}
			if len(executor.calls) != 0 {
				t.Fatalf("hidden tool executed: %v", executor.calls)
			}
			if len(result.ToolResults) != 0 {
				t.Fatalf("hidden tool produced public results: %v", result.ToolResults)
			}
		})
	}
}

func TestActiveTools_EmptyStateSurvivesOptionsAndBuilders(t *testing.T) {
	var optionRequest GenerateTextRequest
	WithActiveTools()(&optionRequest)
	if optionRequest.ActiveTools == nil {
		t.Fatal("WithActiveTools() collapsed an explicit empty allowlist to nil")
	}

	built := NewRequest(nil, "test").ActiveTools().Build()
	if built.ActiveTools == nil {
		t.Fatal("RequestBuilder.ActiveTools() collapsed an explicit empty allowlist to nil")
	}

	cloned := FromRequest(GenerateTextRequest{ActiveTools: []string{}}).Build()
	if cloned.ActiveTools == nil {
		t.Fatal("FromRequest collapsed an explicit empty allowlist to nil")
	}
}

func TestPrepareStepModelOverrideRetainsMiddleware(t *testing.T) {
	tests := []struct {
		name string
		run  func(
			context.Context,
			LanguageModel,
			*ToolSet,
			PrepareStepFunc,
			LanguageModelMiddleware,
		) (*GenerateTextResult, error)
	}{
		{
			name: "bare",
			run: func(
				ctx context.Context,
				model LanguageModel,
				tools *ToolSet,
				prepare PrepareStepFunc,
				middleware LanguageModelMiddleware,
			) (*GenerateTextResult, error) {
				return GenerateText(ctx, GenerateTextRequest{
					Model:       model,
					Tools:       tools,
					PrepareStep: prepare,
					Middlewares: []LanguageModelMiddleware{middleware},
					StopWhen:    IsStepCount(2),
					MaxSteps:    2,
				})
			},
		},
		{
			name: "runtime",
			run: func(
				ctx context.Context,
				model LanguageModel,
				tools *ToolSet,
				prepare PrepareStepFunc,
				middleware LanguageModelMiddleware,
			) (*GenerateTextResult, error) {
				runtime := NewRuntime(WithDefaultModel(model))
				return runtime.GenerateText(
					ctx,
					"test",
					WithTools(tools),
					WithPrepareStep(prepare),
					WithMiddleware(middleware),
					WithStopWhen(IsStepCount(2)),
					WithMaxSteps(2),
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mainModel := &requestRecordingModel{calls: [][]StreamEvent{{
				{
					Type:              StreamEventToolCallDelta,
					ToolCallIndex:     0,
					ToolCallID:        "call-1",
					ToolCallName:      "search",
					ToolCallArgsDelta: `{}`,
				},
				{Type: StreamEventFinish, FinishReason: FinishReasonToolCalls},
			}}}
			alternateModel := &requestRecordingModel{calls: [][]StreamEvent{{
				{Type: StreamEventTextDelta, TextDelta: "done"},
				{Type: StreamEventFinish, FinishReason: FinishReasonStop},
			}}}
			tools := &ToolSet{
				Definitions: []ToolDefinition{{Name: "search"}},
				Executor:    testToolExecutor{},
			}
			streamCalls := 0
			_, err := test.run(
				context.Background(),
				mainModel,
				tools,
				func(ctx PrepareStepContext) *PrepareStepResult {
					if ctx.StepNumber == 1 {
						return &PrepareStepResult{Model: alternateModel}
					}
					return nil
				},
				countModelStreams(&streamCalls),
			)
			if err != nil {
				t.Fatalf("GenerateText: %v", err)
			}
			if got, want := streamCalls, 2; got != want {
				t.Fatalf("middleware observed %d model streams, want %d", got, want)
			}
		})
	}
}
