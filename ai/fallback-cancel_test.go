package ai_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/ai"
)

// stalledModel opens a stream that never emits and never closes, modelling a
// primary that hangs after the HTTP response headers arrive.
type stalledModel struct{}

func (stalledModel) ModelID() string { return "stalled" }

func (stalledModel) Stream(_ context.Context, _ ai.LanguageModelRequest) (<-chan ai.StreamEvent, error) {
	return make(chan ai.StreamEvent), nil // never sends, never closes
}

// TestWithFallback_StalledPrimaryHonoursCancel verifies that a stalled primary
// does not pin the caller past its own deadline: once the context is cancelled,
// WithFallback returns promptly with the cancellation error.
func TestWithFallback_StalledPrimaryHonoursCancel(t *testing.T) {
	fb := ai.WithFallback(stalledModel{}, stalledModel{})
	ctx, cancel := context.WithCancel(context.Background())

	type result struct {
		err error
	}
	done := make(chan result, 1)
	go func() {
		_, err := fb.Stream(ctx, ai.LanguageModelRequest{})
		done <- result{err}
	}()

	// The primary is stalled; only cancellation can unblock the first-event wait.
	cancel()

	select {
	case r := <-done:
		if !errors.Is(r.err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", r.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WithFallback did not honour cancellation while waiting on a stalled primary")
	}
}
