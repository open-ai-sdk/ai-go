package ai_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
	"github.com/open-ai-sdk/ai-go/tool"
)

type typedToolModel struct {
	step int
}

func (m *typedToolModel) ModelID() string { return "typed-tool" }

func (m *typedToolModel) Stream(
	_ context.Context,
	_ ai.LanguageModelRequest,
) (<-chan ai.StreamEvent, error) {
	m.step++
	events := make(chan ai.StreamEvent, 3)
	if m.step == 1 {
		events <- ai.StreamEvent{
			Type:              ai.StreamEventToolCallDelta,
			ToolCallIndex:     0,
			ToolCallID:        "call-sum",
			ToolCallName:      "sum",
			ToolCallArgsDelta: `{"a":2,"b":3}`,
		}
		events <- ai.StreamEvent{
			Type:         ai.StreamEventFinish,
			FinishReason: ai.FinishReasonToolCalls,
		}
	} else {
		events <- ai.StreamEvent{
			Type:         ai.StreamEventFinish,
			FinishReason: ai.FinishReasonStop,
		}
	}
	close(events)
	return events, nil
}

func TestTypedToolRunsThroughAgentLoop(t *testing.T) {
	type sumInput struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	type sumOutput struct {
		Sum int `json:"sum"`
	}

	var gotCallID string
	var gotToolContext string
	var gotRuntimeContext string
	sum, err := tool.New(
		"sum",
		"Add two integers",
		func(ctx context.Context, input sumInput) (sumOutput, error) {
			gotCallID = tool.ToolCallIDFromContext(ctx)
			gotToolContext, _ = tool.TypedContext[string](ctx)
			gotRuntimeContext, _ = tool.RuntimeContextFrom(ctx)["request"].(string)
			return sumOutput{Sum: input.A + input.B}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := tool.NewSet(sum)
	if err != nil {
		t.Fatal(err)
	}

	result, err := ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model:          &typedToolModel{},
		Messages:       []ai.Message{ai.UserMessage("sum")},
		Tools:          tools,
		ToolsContext:   ai.ToolsContext{"sum": "tenant-acme"},
		RuntimeContext: ai.RuntimeContext{"request": "req-1"},
		StopWhen:       ai.IsStepCount(2),
		MaxSteps:       2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ToolResults) != 1 {
		t.Fatalf("tool results = %d, want 1", len(result.ToolResults))
	}
	var output sumOutput
	if err := json.Unmarshal([]byte(result.ToolResults[0].Output), &output); err != nil {
		t.Fatalf("output = %q: %v", result.ToolResults[0].Output, err)
	}
	if output.Sum != 5 {
		t.Errorf("sum = %d, want 5", output.Sum)
	}
	if gotCallID != "call-sum" {
		t.Errorf("call ID = %q, want call-sum", gotCallID)
	}
	if gotToolContext != "tenant-acme" {
		t.Errorf("tool context = %q", gotToolContext)
	}
	if gotRuntimeContext != "req-1" {
		t.Errorf("runtime context = %q", gotRuntimeContext)
	}
}
