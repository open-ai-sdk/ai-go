package aikit_test

import (
	"context"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

type rawUsageModel struct{}

func (rawUsageModel) ModelID() string { return "raw-usage-test" }

func (rawUsageModel) Stream(
	_ context.Context,
	_ llm.Request,
) (<-chan aikit.StreamEvent, error) {
	events := make(chan aikit.StreamEvent, 2)
	events <- aikit.StreamEvent{
		Type: aikit.StreamEventUsage,
		Usage: &aikit.Usage{
			InputTokens:  3,
			OutputTokens: 5,
			TotalTokens:  8,
			Raw: map[string]any{
				"provider": "preserved",
				"cached":   float64(2),
			},
		},
	}
	events <- aikit.StreamEvent{
		Type:         aikit.StreamEventFinish,
		FinishReason: aikit.FinishReasonStop,
	}
	close(events)
	return events, nil
}

func TestUsageRawReachesCompletionResponse(t *testing.T) {
	result, err := llm.Complete(context.Background(), rawUsageModel{}, llm.Request{})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if got := result.Usage.Raw["provider"]; got != "preserved" {
		t.Fatalf("Usage.Raw[provider] = %#v, want preserved", got)
	}
	if got := result.Usage.Raw["cached"]; got != float64(2) {
		t.Fatalf("Usage.Raw[cached] = %#v, want 2", got)
	}
}
