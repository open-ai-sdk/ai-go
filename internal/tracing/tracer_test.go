package tracing

import (
	"context"
	"errors"
	"testing"
)

func TestNewTracerIsSafeNoOp(t *testing.T) {
	ctx := context.Background()
	got, span := NewTracer().Start(ctx, "ai.run", Attr{Key: "ai.step", Value: 1})
	if got != ctx {
		t.Fatal("no-op tracer must preserve context identity")
	}
	span.SetAttributes(Attr{Key: "ai.finish_reason", Value: "stop"})
	span.RecordError(errors.New("ignored"))
	span.End()
}
