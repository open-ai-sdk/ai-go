package generate

import (
	"context"
	"testing"
)

// countingMiddleware wraps a model and counts how many times it wraps, so a
// double-apply would be observable.
func countingMiddleware(calls *int) LanguageModelMiddleware {
	return func(inner LanguageModel) LanguageModel {
		*calls++
		return inner
	}
}

// TestStreamText_BarePathHonoursMiddlewares proves req.Middlewares set on a
// directly-built request are applied exactly once — the fix for the bare path
// that previously ignored them (so WithRetry/WithMiddleware silently did nothing
// unless routed through the Runtime facade).
func TestStreamText_BarePathHonoursMiddlewares(t *testing.T) {
	wraps := 0
	req := GenerateTextRequest{
		Model:       textStreamModel{deltas: []string{"hi"}},
		Messages:    []Message{UserMessage("x")},
		MaxSteps:    1,
		Middlewares: []LanguageModelMiddleware{countingMiddleware(&wraps)},
	}
	if _, err := StreamText(context.Background(), req).Consume(); err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if wraps != 1 {
		t.Fatalf("middleware applied %d times on the bare path, want exactly 1", wraps)
	}
}
