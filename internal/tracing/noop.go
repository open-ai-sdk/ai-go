package tracing

import "context"

// NoopTracer starts no real span: Start is a plain function call that returns
// ctx unchanged and a Span whose methods do nothing. It has no OTel import.
//
// The agent runtime uses it whenever the caller does not inject a tracer.
type NoopTracer struct{}

// Start implements Tracer.
func (NoopTracer) Start(ctx context.Context, _ string, _ ...Attr) (context.Context, Span) {
	return ctx, noopSpan{}
}

type noopSpan struct{}

func (noopSpan) End()                  {}
func (noopSpan) SetAttributes(...Attr) {}
func (noopSpan) RecordError(error)     {}
