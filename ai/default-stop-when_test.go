package ai_test

import (
	"context"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/ai"
)

// alwaysToolCallStepModel emits a tool call on every step, so a test can
// distinguish "the loop stopped because the default stop condition fired"
// from "the loop stopped because the model ran out of tool calls".
type alwaysToolCallStepModel struct{ calls int }

func (m *alwaysToolCallStepModel) ModelID() string { return "always-tool-call-step" }

func (m *alwaysToolCallStepModel) Stream(
	_ context.Context,
	_ ai.LanguageModelRequest,
) (<-chan ai.StreamEvent, error) {
	m.calls++
	ch := make(chan ai.StreamEvent, 2)
	ch <- ai.StreamEvent{
		Type:              ai.StreamEventToolCallDelta,
		ToolCallID:        "tc",
		ToolCallName:      "noop",
		ToolCallArgsDelta: `{}`,
	}
	ch <- ai.StreamEvent{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonToolCalls}
	close(ch)
	return ch, nil
}

type noopStepExecutor struct{}

func (noopStepExecutor) Execute(_ context.Context, _, _ string) (string, error) {
	return `{"ok":true}`, nil
}

func noopStepToolSet() *ai.ToolSet {
	return &ai.ToolSet{
		Definitions: []ai.ToolDefinition{{Name: "noop", InputSchema: map[string]any{"type": "object"}}},
		Executor:    noopStepExecutor{},
	}
}

// TestGenerateText_DefaultStopWhen_PerformsExactlyOneStep proves GenerateText
// with neither MaxSteps nor StopWhen set now performs exactly one step (node
// parity — generateText defaults to stopWhen=isStepCount(1)), not the old
// implicit ten steps the engine used to fall back to. A context timeout
// guards against a regression turning this into an unbounded loop (the fake
// model always calls a tool) instead of a clean test failure.
func TestGenerateText_DefaultStopWhen_PerformsExactlyOneStep(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := &alwaysToolCallStepModel{}
	result, err := ai.GenerateText(ctx, ai.GenerateTextRequest{
		Model:    model,
		Messages: []ai.Message{ai.UserMessage("go")},
		Tools:    noopStepToolSet(),
	})
	if err != nil {
		t.Fatalf("GenerateText: %v", err)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("Steps = %d, want 1 (default stopWhen=IsStepCount(1))", len(result.Steps))
	}
	if model.calls != 1 {
		t.Errorf("model called %d times, want 1", model.calls)
	}
}

// TestStreamText_DefaultStopWhen_PerformsExactlyOneStep is the streaming
// counterpart: StreamText shares GenerateText's default via the same
// toEngineParams conversion, so Consume() must observe the same one-step result.
func TestStreamText_DefaultStopWhen_PerformsExactlyOneStep(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := &alwaysToolCallStepModel{}
	sr := ai.StreamText(ctx, ai.GenerateTextRequest{
		Model:    model,
		Messages: []ai.Message{ai.UserMessage("go")},
		Tools:    noopStepToolSet(),
	})
	result, err := sr.Consume()
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("Steps = %d, want 1", len(result.Steps))
	}
}
