// Package tracing gives the tool loop one small interface for span
// instrumentation so the OpenTelemetry dependency stays confined to this
// file. Swapping OTel for another backend, or dropping it entirely, is a
// single-file change — the agent runtime only sees Tracer/Span/Attr.
package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// instrumentationName identifies this module's spans to whatever backend the
// consumer's application registers as the global OTel TracerProvider.
const instrumentationName = "github.com/open-ai-sdk/ai-go"

// Attr is a span attribute. It exists so every other file in the module can
// describe span data without importing go.opentelemetry.io/otel/attribute.
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

// NewTracer returns a Tracer bound to the process-global OTel TracerProvider.
// Until the consumer's application calls otel.SetTracerProvider with a real
// SDK, that global provider is OTel's own no-op implementation: Start costs
// one interface call and returns a span that discards everything, so this is
// safe to use unconditionally without any SDK-side configuration. If the
// application registers a provider later (the common case — tracing SDKs are
// wired up in main, after package init), OTel's global proxy delegates to it
// automatically; a Tracer obtained here does not need to be re-created.
func NewTracer() Tracer {
	return otelTracer{t: otel.Tracer(instrumentationName)}
}

// otelTracer adapts the OTel trace API to Tracer. Confined to this file so
// the rest of the module never imports go.opentelemetry.io directly.
type otelTracer struct{ t trace.Tracer }

func (o otelTracer) Start(ctx context.Context, name string, attrs ...Attr) (context.Context, Span) {
	ctx, span := o.t.Start(ctx, name, trace.WithAttributes(toKeyValues(attrs)...))
	return ctx, otelSpan{span}
}

type otelSpan struct{ s trace.Span }

func (o otelSpan) End() { o.s.End() }

func (o otelSpan) SetAttributes(attrs ...Attr) { o.s.SetAttributes(toKeyValues(attrs)...) }

func (o otelSpan) RecordError(err error) {
	if err == nil {
		return
	}
	o.s.RecordError(err)
}

// toKeyValues converts the package's provider-agnostic Attr into OTel's
// attribute.KeyValue. An unrecognized Value type falls back to fmt.Sprint
// rather than dropping the attribute silently.
func toKeyValues(attrs []Attr) []attribute.KeyValue {
	if len(attrs) == 0 {
		return nil
	}
	kv := make([]attribute.KeyValue, len(attrs))
	for i, a := range attrs {
		switch v := a.Value.(type) {
		case string:
			kv[i] = attribute.String(a.Key, v)
		case int:
			kv[i] = attribute.Int(a.Key, v)
		case int64:
			kv[i] = attribute.Int64(a.Key, v)
		case bool:
			kv[i] = attribute.Bool(a.Key, v)
		default:
			kv[i] = attribute.String(a.Key, fmt.Sprint(v))
		}
	}
	return kv
}
