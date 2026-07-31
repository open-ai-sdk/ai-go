package tracing

import "context"

// NoopTracer starts no real span: Start is a plain function call that returns
// ctx unchanged and a Span whose methods do nothing. It has no OTel import.
//
// The agent runtime falls back to it only through its package-private disabled
// instrumentation seam, used by tests and benchmarks. Public runs bind
// tracing.NewTracer() (OTel's global, itself a no-op until the application
// registers a provider).
type NoopTracer struct{}

// Start implements Tracer.
func (NoopTracer) Start(ctx context.Context, _ string, _ ...Attr) (context.Context, Span) {
	return ctx, noopSpan{}
}

type noopSpan struct{}

func (noopSpan) End()                  {}
func (noopSpan) SetAttributes(...Attr) {}
func (noopSpan) RecordError(error)     {}
