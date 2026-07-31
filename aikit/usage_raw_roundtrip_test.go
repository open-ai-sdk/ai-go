package aikit_test

import (
	"context"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
	"github.com/open-ai-sdk/ai-go/aikit"
)

type rawUsageModel struct{}

func (rawUsageModel) ModelID() string { return "raw-usage-test" }

func (rawUsageModel) Stream(
	_ context.Context,
	_ aikit.ModelRequest,
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

func TestUsageRawReachesGenerateTextResult(t *testing.T) {
	result, err := ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model: rawUsageModel{},
	})
	if err != nil {
		t.Fatalf("GenerateText() error = %v", err)
	}
	if got := result.Usage.Raw["provider"]; got != "preserved" {
		t.Fatalf("Usage.Raw[provider] = %#v, want preserved", got)
	}
	if got := result.FinalStep.Usage.Raw["cached"]; got != float64(2) {
		t.Fatalf("FinalStep.Usage.Raw[cached] = %#v, want 2", got)
	}
}
