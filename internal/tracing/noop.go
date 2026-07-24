package tracing

import "context"

// NoopTracer starts no real span: Start is a plain function call that
// returns ctx unchanged and a Span whose methods do nothing. internal/engine
// falls back to this when no Tracer was configured, so a caller that never
// opts into tracing pays no OTel cost at all — not even the global-provider
// no-op path in tracer.go, since this file has no OTel import at all.
type NoopTracer struct{}

// Start implements Tracer.
func (NoopTracer) Start(ctx context.Context, _ string, _ ...Attr) (context.Context, Span) {
	return ctx, noopSpan{}
}

type noopSpan struct{}

func (noopSpan) End()                  {}
func (noopSpan) SetAttributes(...Attr) {}
func (noopSpan) RecordError(error)     {}
