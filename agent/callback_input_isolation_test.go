package agent

import (
	"context"
	"testing"
)

func TestPrepareStep_CompletedStepsAreDefensiveSnapshots(t *testing.T) {
	model := &mockModel{calls: [][]StreamEvent{
		{
			textEvt("original"),
			{Type: StreamEventUsage, Usage: &Usage{
				InputTokens: 1,
				TotalTokens: 1,
				Raw: map[string]any{
					"nested": map[string]any{"value": "original"},
				},
			}},
			toolCallEvt(0, "call-1", "search", `{"q":"go"}`),
			{
				Type:             StreamEventFinish,
				FinishReason:     FinishReasonToolCalls,
				ProviderMetadata: map[string]any{"nested": map[string]any{"value": "original"}},
			},
		},
		{textEvt("done"), finishEvt(FinishReasonStop)},
	}}
	executor := &mockExecutor{}

	var capturedEnd endEvent
	channel := driveStream(context.Background(), runConfig{
		Model: model,
		Tools: testToolSet(
			[]ToolDefinition{{Name: "search"}},
			executor),

		PrepareStep: func(ctx PrepareStepContext) *PrepareStepResult {
			if ctx.StepNumber != 1 {
				return nil
			}
			step := &ctx.Steps[0]
			step.Text = "corrupt"
			step.ToolNames[0] = "corrupt"
			step.ToolCalls[0].Args[0] = 'X'
			step.Usage.InputTokens = 999
			step.Usage.Raw["nested"].(map[string]any)["value"] = "corrupt"
			step.ProviderMetadata["nested"].(map[string]any)["value"] = "corrupt"
			return nil
		},
		Callbacks: &lifecycleCallbacks{
			OnEnd: func(event endEvent) {
				capturedEnd = event
			},
		},
		MaxSteps: 2,
	})
	for event := range channel {
		if event.Type == StepEventError {
			t.Fatalf("unexpected error: %v", event.Error)
		}
	}

	step := capturedEnd.Steps[0]
	if step.Text != "original" {
		t.Fatalf("OnEnd step text = %q, want original", step.Text)
	}
	if step.ToolNames[0] != "search" {
		t.Fatalf("OnEnd tool name = %q, want search", step.ToolNames[0])
	}
	if got := string(step.ToolCalls[0].Args); got != `{"q":"go"}` {
		t.Fatalf("OnEnd tool args = %q, want original JSON", got)
	}
	if step.Usage.InputTokens != 1 {
		t.Fatalf("OnEnd input tokens = %d, want 1", step.Usage.InputTokens)
	}
	if got := step.Usage.Raw["nested"].(map[string]any)["value"]; got != "original" {
		t.Fatalf("OnEnd Raw value = %v, want original", got)
	}
	if got := step.ProviderMetadata["nested"].(map[string]any)["value"]; got != "original" {
		t.Fatalf("OnEnd provider metadata value = %v, want original", got)
	}
}

func TestRepairToolCall_ReceivesIsolatedToolDefinitions(t *testing.T) {
	definitions := []ToolDefinition{{
		Name:          "search",
		InputSchema:   map[string]any{"nested": map[string]any{"value": "original"}},
		ContextSchema: map[string]any{"nested": map[string]any{"value": "original"}},
	}}
	executor := &mockExecutor{}
	model := &mockModel{calls: [][]StreamEvent{{
		toolCallEvt(0, "call-1", "Search", `{}`),
		finishEvt(FinishReasonToolCalls),
	}}}

	channel := driveStream(context.Background(), runConfig{
		Model:   model,
		Request: Request{Tools: definitions},
		Tools:   testToolSet(definitions, executor),
		RepairToolCall: func(
			_ context.Context,
			input ToolCallRepairContext,
		) (*ToolCallInfo, error) {
			input.Tools[0].Name = "admin"
			input.Tools[0].InputSchema["nested"].(map[string]any)["value"] = "corrupt"
			input.Tools[0].ContextSchema["nested"].(map[string]any)["value"] = "corrupt"
			return &ToolCallInfo{Name: "search"}, nil
		},
		MaxSteps: 1,
	})
	for event := range channel {
		if event.Type == StepEventError {
			t.Fatalf("unexpected error: %v", event.Error)
		}
	}

	if len(executor.called) != 1 || executor.called[0] != "search" {
		t.Fatalf("executor calls = %v, want [search]", executor.called)
	}
	if definitions[0].Name != "search" {
		t.Fatalf("original definition name = %q, want search", definitions[0].Name)
	}
	if got := definitions[0].InputSchema["nested"].(map[string]any)["value"]; got != "original" {
		t.Fatalf("original input schema value = %v, want original", got)
	}
	if got := definitions[0].ContextSchema["nested"].(map[string]any)["value"]; got != "original" {
		t.Fatalf("original context schema value = %v, want original", got)
	}
}

func TestOnChunk_UsagePayloadDoesNotAliasStreamEvent(t *testing.T) {
	model := &mockModel{calls: [][]StreamEvent{{
		{
			Type: StreamEventUsage,
			Usage: &Usage{
				InputTokens: 1,
				Raw: map[string]any{
					"nested": map[string]any{"value": "original"},
				},
			},
		},
		finishEvt(FinishReasonStop),
	}}}

	channel := driveStream(context.Background(), runConfig{
		Model: model,
		Callbacks: &lifecycleCallbacks{
			OnChunk: func(event StepEvent) {
				if event.Type != StepEventUsage {
					return
				}
				event.Usage.InputTokens = 999
				event.Usage.Raw["nested"].(map[string]any)["value"] = "corrupt"
			},
		},
		MaxSteps: 1,
	})
	for event := range channel {
		if event.Type != StepEventUsage {
			continue
		}
		if event.Usage.InputTokens != 1 {
			t.Fatalf("stream usage InputTokens = %d, want 1", event.Usage.InputTokens)
		}
		if got := event.Usage.Raw["nested"].(map[string]any)["value"]; got != "original" {
			t.Fatalf("stream usage Raw value = %v, want original", got)
		}
	}
}
