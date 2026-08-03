package otelagent

import (
	"context"
	"fmt"

	"github.com/open-ai-sdk/ai-go/agent"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/open-ai-sdk/ai-go"

// New adapts tracer for [agent.Builder.Tracer] and [agent.Runner.Tracer].
func New(tracer trace.Tracer) agent.Tracer { return adapter{tracer: tracer} }

// Global adapts the process-global OpenTelemetry provider.
func Global() agent.Tracer { return New(otel.Tracer(instrumentationName)) }

type adapter struct{ tracer trace.Tracer }

func (a adapter) Start(
	ctx context.Context,
	name string,
	attrs ...agent.Attr,
) (context.Context, agent.Span) {
	ctx, span := a.tracer.Start(ctx, name, trace.WithAttributes(keyValues(attrs)...))
	return ctx, wrappedSpan{span: span}
}

type wrappedSpan struct{ span trace.Span }

func (s wrappedSpan) End() { s.span.End() }

func (s wrappedSpan) SetAttributes(attrs ...agent.Attr) {
	s.span.SetAttributes(keyValues(attrs)...)
}

func (s wrappedSpan) RecordError(err error) {
	if err != nil {
		s.span.RecordError(err)
	}
}

func keyValues(attrs []agent.Attr) []attribute.KeyValue {
	if len(attrs) == 0 {
		return nil
	}
	values := make([]attribute.KeyValue, len(attrs))
	for index, attr := range attrs {
		switch value := attr.Value.(type) {
		case string:
			values[index] = attribute.String(attr.Key, value)
		case int:
			values[index] = attribute.Int(attr.Key, value)
		case int64:
			values[index] = attribute.Int64(attr.Key, value)
		case bool:
			values[index] = attribute.Bool(attr.Key, value)
		default:
			values[index] = attribute.String(attr.Key, fmt.Sprint(value))
		}
	}
	return values
}
