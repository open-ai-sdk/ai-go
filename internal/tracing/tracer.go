// Package tracing defines the provider-neutral instrumentation seam used by
// the agent runtime. The core module intentionally ships no tracing backend.
package tracing

import "context"

// Attr is a span attribute.
type Attr struct {
	Key   string
	Value any
}

// Span is the subset of span behavior the tool loop needs.
type Span interface {
	End()
	SetAttributes(attrs ...Attr)
	RecordError(err error)
}

// Tracer starts spans for tool-loop instrumentation.
type Tracer interface {
	Start(ctx context.Context, name string, attrs ...Attr) (context.Context, Span)
}

// NewTracer returns the core module's no-op tracer. Applications opt into a
// backend through the public Agent Builder or Runner (normally through an
// adapter module); importing ai-go never activates a telemetry SDK.
func NewTracer() Tracer { return NoopTracer{} }
