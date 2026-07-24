package ai_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
)

// accumulatingModel drives a two-step tool loop while emitting per-step usage
// and a distinct source on each step, so the aggregated result can be checked:
// step 1 calls a tool (usage 10/5, source S1), step 2 emits text and stops
// (usage 20/8, source S2).
type accumulatingModel struct {
	step int
}

func (m *accumulatingModel) ModelID() string { return "accumulating" }

func (m *accumulatingModel) Stream(
	_ context.Context,
	_ ai.LanguageModelRequest,
) (<-chan ai.StreamEvent, error) {
	m.step++
	ch := make(chan ai.StreamEvent, 8)
	if m.step == 1 {
		ch <- ai.StreamEvent{
			Type:              ai.StreamEventToolCallDelta,
			ToolCallIndex:     0,
			ToolCallID:        "tc-1",
			ToolCallName:      "add",
			ToolCallArgsDelta: `{"a":1,"b":2}`,
		}
		ch <- ai.StreamEvent{Type: ai.StreamEventSource, Source: &ai.Source{SourceType: "url", ID: "S1", URL: "https://a.example"}}
		ch <- ai.StreamEvent{Type: ai.StreamEventUsage, Usage: &ai.Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}}
		ch <- ai.StreamEvent{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonToolCalls}
	} else {
		ch <- ai.StreamEvent{Type: ai.StreamEventTextDelta, TextDelta: "The answer is 3."}
		ch <- ai.StreamEvent{Type: ai.StreamEventSource, Source: &ai.Source{SourceType: "url", ID: "S2", URL: "https://b.example"}}
		ch <- ai.StreamEvent{Type: ai.StreamEventUsage, Usage: &ai.Usage{InputTokens: 20, OutputTokens: 8, TotalTokens: 28}}
		ch <- ai.StreamEvent{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonStop}
	}
	close(ch)
	return ch, nil
}

type addExec struct{}

func (addExec) Execute(_ context.Context, _, argsJSON string) (string, error) {
	var args struct {
		A, B int
	}
	_ = json.Unmarshal([]byte(argsJSON), &args)
	out, _ := json.Marshal(map[string]int{"result": args.A + args.B})
	return string(out), nil
}

// TestGenerateText_ResultAccumulationAndFinalStep verifies that a multi-step run
// unions tool results and sources at the top level, sums usage across steps, and
// exposes the last step as FinalStep.
func TestGenerateText_ResultAccumulationAndFinalStep(t *testing.T) {
	result, err := ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model:    &accumulatingModel{},
		Messages: []ai.Message{ai.UserMessage("What is 1+2?")},
		Tools: &ai.ToolSet{
			Definitions: []ai.ToolDefinition{{Name: "add", InputSchema: map[string]any{"type": "object"}}},
			Executor:    addExec{},
		},
		MaxSteps: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result.Steps))
	}

	// Usage is summed across both steps.
	if result.Usage.InputTokens != 30 {
		t.Errorf("total InputTokens = %d, want 30", result.Usage.InputTokens)
	}
	if result.Usage.OutputTokens != 13 {
		t.Errorf("total OutputTokens = %d, want 13", result.Usage.OutputTokens)
	}
	if result.Usage.TotalTokens != 43 {
		t.Errorf("total TotalTokens = %d, want 43", result.Usage.TotalTokens)
	}

	// Sources union across steps.
	if len(result.Sources) != 2 {
		t.Errorf("expected 2 sources unioned, got %d", len(result.Sources))
	}

	// Tool results union across steps (one add call in step 1).
	if len(result.ToolResults) != 1 {
		t.Errorf("expected 1 tool result, got %d", len(result.ToolResults))
	}

	// FinalStep is the last step: text + stop, with that step's own usage only.
	if result.FinalStep.Text != "The answer is 3." {
		t.Errorf("FinalStep.Text = %q, want %q", result.FinalStep.Text, "The answer is 3.")
	}
	if result.FinalStep.FinishReason != ai.FinishReasonStop {
		t.Errorf("FinalStep.FinishReason = %s, want stop", result.FinalStep.FinishReason)
	}
	if result.FinalStep.Usage.InputTokens != 20 || result.FinalStep.Usage.OutputTokens != 8 {
		t.Errorf("FinalStep.Usage = %d/%d, want 20/8", result.FinalStep.Usage.InputTokens, result.FinalStep.Usage.OutputTokens)
	}
}
