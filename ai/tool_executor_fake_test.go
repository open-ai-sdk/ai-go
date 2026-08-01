package ai_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
)

// fakeToolExecutor is a minimal, in-tree stand-in for a consumer's tool
// dispatcher. It exists to prove ai.ToolExecutor and ai.StreamingToolExecutor
// are the only interfaces a dispatcher needs to satisfy to drive a full tool
// loop — nothing here is wired into the SDK itself.
type fakeToolExecutor struct {
	calls int32 // mutated from tool calls the engine may run concurrently
}

var (
	_ ai.ToolExecutor          = (*fakeToolExecutor)(nil)
	_ ai.StreamingToolExecutor = (*fakeToolExecutor)(nil)
)

func (f *fakeToolExecutor) Execute(_ context.Context, name, argsJSON string) (string, error) {
	atomic.AddInt32(&f.calls, 1)
	if name != "lookup" {
		return `{"ok":true}`, nil
	}
	var args struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", err
	}
	return `{"value":"` + args.Key + `-resolved"}`, nil
}

func (f *fakeToolExecutor) ExecuteStreaming(
	ctx context.Context,
	name, argsJSON string,
	stream ai.ToolResultStream,
) (string, error) {
	stream.Write("partial:" + name)
	return f.Execute(ctx, name, argsJSON)
}

// fakeMultiStepModel drives a three-step tool loop: steps 1 and 2 each call a
// different tool, step 3 emits text and stops.
type fakeMultiStepModel struct{ step int }

func (m *fakeMultiStepModel) ModelID() string { return "fake-multi-step" }

func (m *fakeMultiStepModel) Stream(
	_ context.Context,
	_ ai.LanguageModelRequest,
) (<-chan ai.StreamEvent, error) {
	m.step++
	ch := make(chan ai.StreamEvent, 4)
	switch m.step {
	case 1:
		ch <- ai.StreamEvent{
			Type:              ai.StreamEventToolCallDelta,
			ToolCallID:        "tc-1",
			ToolCallName:      "lookup",
			ToolCallArgsDelta: `{"key":"a"}`,
		}
		ch <- ai.StreamEvent{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonToolCalls}
	case 2:
		ch <- ai.StreamEvent{
			Type:              ai.StreamEventToolCallDelta,
			ToolCallID:        "tc-2",
			ToolCallName:      "noop",
			ToolCallArgsDelta: `{}`,
		}
		ch <- ai.StreamEvent{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonToolCalls}
	default:
		ch <- ai.StreamEvent{Type: ai.StreamEventTextDelta, TextDelta: "done"}
		ch <- ai.StreamEvent{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonStop}
	}
	close(ch)
	return ch, nil
}

// TestToolLoop_FakeExecutor_DrivesMultiStep proves a full tool loop runs end
// to end against nothing but the public ToolExecutor seam: no in-tree
// executor type is required to drive GenerateText, only the interface.
func TestToolLoop_FakeExecutor_DrivesMultiStep(t *testing.T) {
	model := &fakeMultiStepModel{}
	executor := &fakeToolExecutor{}

	result, err := ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model:    model,
		Messages: []ai.Message{ai.UserMessage("resolve a then noop")},
		Tools: &ai.ToolSet{
			Definitions: []ai.ToolDefinition{
				{Name: "lookup", InputSchema: map[string]any{"type": "object"}},
				{Name: "noop", InputSchema: map[string]any{"type": "object"}},
			},
			Executor: executor,
		},
		// GenerateText defaults StopWhen to IsStepCount(1); this test exercises
		// a real three-step loop, so it must opt in explicitly.
		StopWhen: ai.Never(),
		MaxSteps: 5,
	})
	if err != nil {
		t.Fatalf("GenerateText: %v", err)
	}
	if result.Text != "done" {
		t.Errorf("Text = %q, want %q", result.Text, "done")
	}
	if len(result.Steps) != 3 {
		t.Fatalf("Steps = %d, want 3", len(result.Steps))
	}
	if got := atomic.LoadInt32(&executor.calls); got != 2 {
		t.Errorf("executor.calls = %d, want 2", got)
	}
	if len(result.ToolResults) != 2 {
		t.Fatalf("ToolResults = %d, want 2", len(result.ToolResults))
	}
	if result.ToolResults[0].Output != `{"value":"a-resolved"}` {
		t.Errorf("first tool result = %q", result.ToolResults[0].Output)
	}
}
