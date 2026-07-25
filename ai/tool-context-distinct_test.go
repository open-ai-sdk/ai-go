package ai_test

import (
	"context"
	"sync"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
)

// twoToolModel calls two distinct tools in the first step, then stops.
type twoToolModel struct {
	step int
}

func (m *twoToolModel) ModelID() string { return "two-tool" }

func (m *twoToolModel) Stream(
	_ context.Context,
	_ ai.LanguageModelRequest,
) (<-chan ai.StreamEvent, error) {
	m.step++
	ch := make(chan ai.StreamEvent, 8)
	if m.step == 1 {
		ch <- ai.StreamEvent{Type: ai.StreamEventToolCallDelta, ToolCallIndex: 0, ToolCallID: "a-1", ToolCallName: "alpha", ToolCallArgsDelta: `{}`}
		ch <- ai.StreamEvent{Type: ai.StreamEventToolCallDelta, ToolCallIndex: 1, ToolCallID: "b-1", ToolCallName: "beta", ToolCallArgsDelta: `{}`}
		ch <- ai.StreamEvent{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonToolCalls}
	} else {
		ch <- ai.StreamEvent{Type: ai.StreamEventTextDelta, TextDelta: "ok"}
		ch <- ai.StreamEvent{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonStop}
	}
	close(ch)
	return ch, nil
}

// ctxCaptureExecutor records the per-tool context value observed during Execute.
type ctxCaptureExecutor struct {
	mu   sync.Mutex
	seen map[string]any
}

func (e *ctxCaptureExecutor) Execute(ctx context.Context, name, _ string) (string, error) {
	v, _ := ai.ToolContextFrom(ctx)
	e.mu.Lock()
	e.seen[name] = v
	e.mu.Unlock()
	return `{"ok":true}`, nil
}

// TestToolsContext_DistinctToolsGetDistinctContexts verifies that each tool
// receives its own configured context value during execution, and never another
// tool's.
func TestToolsContext_DistinctToolsGetDistinctContexts(t *testing.T) {
	alphaCtx := map[string]any{"tenant": "acme"}
	betaCtx := map[string]any{"tenant": "globex"}

	exec := &ctxCaptureExecutor{seen: map[string]any{}}
	_, err := ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model:    &twoToolModel{},
		Messages: []ai.Message{ai.UserMessage("go")},
		Tools: &ai.ToolSet{
			Definitions: []ai.ToolDefinition{
				{Name: "alpha", InputSchema: map[string]any{"type": "object"}},
				{Name: "beta", InputSchema: map[string]any{"type": "object"}},
			},
			Executor: exec,
		},
		ToolsContext: ai.ToolsContext{
			"alpha": alphaCtx,
			"beta":  betaCtx,
		},
		MaxSteps: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotAlpha, _ := exec.seen["alpha"].(map[string]any)
	gotBeta, _ := exec.seen["beta"].(map[string]any)
	if gotAlpha["tenant"] != "acme" {
		t.Errorf("alpha context = %v, want tenant=acme", exec.seen["alpha"])
	}
	if gotBeta["tenant"] != "globex" {
		t.Errorf("beta context = %v, want tenant=globex", exec.seen["beta"])
	}
	if gotAlpha["tenant"] == gotBeta["tenant"] {
		t.Error("expected distinct tools to receive distinct contexts")
	}
}
