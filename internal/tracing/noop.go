package tracing

import "context"

// NoopTracer starts no real span: Start is a plain function call that returns
// ctx unchanged and a Span whose methods do nothing. It has no OTel import.
//
// The engine falls back to it only when RunParams.Tracer is nil — i.e. when the
// engine is driven directly (internal tests, custom callers) without wiring a
// tracer. The public ai API always wires tracing.NewTracer() (OTel's global,
// which is itself a no-op until the application registers a provider), so this
// fallback is not on the public path; it is defensive nil-safety for the agent.
type NoopTracer struct{}

// Start implements Tracer.
func (NoopTracer) Start(ctx context.Context, _ string, _ ...Attr) (context.Context, Span) {
	return ctx, noopSpan{}
}

type noopSpan struct{}

func (noopSpan) End()                  {}
func (noopSpan) SetAttributes(...Attr) {}
func (noopSpan) RecordError(error)     {}
