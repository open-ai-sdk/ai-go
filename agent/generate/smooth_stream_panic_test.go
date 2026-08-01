package ai

import (
	"context"
	"errors"
	"testing"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/internal/safego"
)

// TestSmoothStream_DetectorPanic_SurfacesErrorEvent verifies that a panic in a
// consumer-supplied chunk detector (a control callback whose return steers the
// re-chunking) is recovered and surfaced as a StepEventError before the output
// channel closes — the process survives and the consumer observes the failure
// instead of a silently truncated stream.
func TestSmoothStream_DetectorPanic_SurfacesErrorEvent(t *testing.T) {
	ss := NewSmoothStream(WithDelayMs(0), WithChunkDetector(
		func(string) (string, string, error) { panic("detector boom") },
	))

	out := ss.Transform(context.Background(), feedEvents(textDelta("hello world")))
	events := collectEvents(t, out)

	var lastErr error
	sawError := false
	for _, ev := range events {
		if ev.Type == agent.StepEventError {
			sawError = true
			lastErr = ev.Error
		}
	}

	if !sawError {
		t.Fatal("expected a StepEventError from the panicking detector")
	}
	var pe *safego.PanicError
	if !errors.As(lastErr, &pe) {
		t.Fatalf("expected *safego.PanicError, got %T (%v)", lastErr, lastErr)
	}
	if len(pe.Stack) == 0 {
		t.Error("expected a non-empty stack trace on the PanicError")
	}
}
