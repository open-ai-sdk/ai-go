package generate

import (
	"context"
	"errors"
	"testing"
)

var errLaterModelStep = errors.New("later model step failed")

type toolThenStartErrorModel struct {
	calls int
}

func (m *toolThenStartErrorModel) ModelID() string { return "tool-then-error" }

func (m *toolThenStartErrorModel) Stream(
	context.Context,
	LanguageModelRequest,
) (<-chan StreamEvent, error) {
	m.calls++
	if m.calls == 2 {
		return nil, errLaterModelStep
	}
	ch := make(chan StreamEvent, 2)
	ch <- StreamEvent{
		Type:              StreamEventToolCallDelta,
		ToolCallIndex:     0,
		ToolCallID:        "tc1",
		ToolCallName:      "lookup",
		ToolCallArgsDelta: `{}`,
	}
	ch <- StreamEvent{Type: StreamEventFinish, FinishReason: FinishReasonToolCalls}
	close(ch)
	return ch, nil
}

type fixedResultExecutor struct{}

func (fixedResultExecutor) Execute(context.Context, string, string) (string, error) {
	return `{"value":42}`, nil
}

type cancelHungModel struct {
	started chan struct{}
}

func (m *cancelHungModel) ModelID() string { return "cancel-hung" }

func (m *cancelHungModel) Stream(
	context.Context,
	LanguageModelRequest,
) (<-chan StreamEvent, error) {
	close(m.started)
	return make(chan StreamEvent), nil
}

func TestGenerateText_PreservesToolResultWhenLaterStepFails(t *testing.T) {
	model := &toolThenStartErrorModel{}
	result, err := GenerateText(context.Background(), GenerateTextRequest{
		Model: model,
		Tools: &ToolSet{
			Definitions: []ToolDefinition{{Name: "lookup"}},
			Executor:    fixedResultExecutor{},
		},
		StopWhen: Never(),
	})

	if !errors.Is(err, errLaterModelStep) {
		t.Fatalf("error = %v, want %v", err, errLaterModelStep)
	}
	if result == nil {
		t.Fatal("partial result is nil")
	}
	if len(result.ToolResults) != 1 || result.ToolResults[0].Output != `{"value":42}` {
		t.Fatalf("tool results = %#v, want completed lookup result", result.ToolResults)
	}
	if len(result.Steps) != 1 || len(result.Steps[0].ToolResults) != 1 {
		t.Fatalf("completed steps = %#v, want first tool step retained", result.Steps)
	}
	if model.calls != 2 {
		t.Fatalf("model calls = %d, want 2", model.calls)
	}
}

func TestGenerateText_ReturnsContextCancellationFromHungProvider(t *testing.T) {
	model := &cancelHungModel{started: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := GenerateText(ctx, GenerateTextRequest{Model: model})
		done <- err
	}()
	<-model.started
	cancel()

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
