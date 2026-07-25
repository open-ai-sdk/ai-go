package tracing

import (
	"context"
	"errors"
	"testing"
)

func TestNoopTracer_StartReturnsCtxUnchanged(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "marker")

	gotCtx, span := (NoopTracer{}).Start(ctx, "ai.run", Attr{Key: "x", Value: 1})

	if gotCtx.Value(key{}) != "marker" {
		t.Fatal("NoopTracer.Start must return ctx unchanged")
	}
	// None of these may panic, and none should have any observable effect.
	span.SetAttributes(Attr{Key: "a", Value: "b"})
	span.RecordError(errors.New("boom"))
	span.End()
}

func TestNoopTracer_ImplementsTracer(t *testing.T) {
	var _ Tracer = NoopTracer{}
	var _ Span = noopSpan{}
}
