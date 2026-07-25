package tracing

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/embedded"
)

// fakeSpan records every call made against it. Embedding embedded.Span
// promotes the (unexported, uncallable) marker method OTel requires of any
// trace.Span implementation; every exported method of trace.Span still has
// to be implemented explicitly below, even the ones this test never asserts
// on, or fakeSpan would not satisfy the interface.
type fakeSpan struct {
	embedded.Span
	ended       bool
	attrs       []attribute.KeyValue
	recordedErr error
}

func (s *fakeSpan) End(...trace.SpanEndOption)                    { s.ended = true }
func (s *fakeSpan) SetAttributes(kv ...attribute.KeyValue)        { s.attrs = append(s.attrs, kv...) }
func (s *fakeSpan) RecordError(err error, _ ...trace.EventOption) { s.recordedErr = err }
func (s *fakeSpan) SetStatus(codes.Code, string)                  {}
func (s *fakeSpan) TracerProvider() trace.TracerProvider          { return nil }
func (s *fakeSpan) AddEvent(string, ...trace.EventOption)         {}
func (s *fakeSpan) AddLink(trace.Link)                            {}
func (s *fakeSpan) IsRecording() bool                             { return true }
func (s *fakeSpan) SpanContext() trace.SpanContext                { return trace.SpanContext{} }
func (s *fakeSpan) SetName(string)                                {}

// fakeTracer records the name each span was started with and hands back a
// single fakeSpan the test can inspect after the call.
type fakeTracer struct {
	embedded.Tracer
	lastName string
	lastSpan *fakeSpan
}

func (t *fakeTracer) Start(
	ctx context.Context,
	name string,
	opts ...trace.SpanStartOption,
) (context.Context, trace.Span) {
	t.lastName = name
	cfg := trace.NewSpanStartConfig(opts...)
	// The real otelTracer passes the Start-time attrs via trace.WithAttributes;
	// unpack the config here the same way an actual SDK tracer would, so this
	// test observes what otelTracer.Start really sent, not just that it called
	// Start.
	t.lastSpan = &fakeSpan{attrs: append([]attribute.KeyValue{}, cfg.Attributes()...)}
	return ctx, t.lastSpan
}

// fakeProvider is a trace.TracerProvider that always returns the same
// fakeTracer, so the test can inspect what the tool loop recorded through it.
type fakeProvider struct {
	embedded.TracerProvider
	tracer *fakeTracer
}

func (p *fakeProvider) Tracer(string, ...trace.TracerOption) trace.Tracer { return p.tracer }

// withFakeProvider registers p as the global OTel TracerProvider for the
// duration of the test and restores whatever was registered before.
func withFakeProvider(t *testing.T, p trace.TracerProvider) {
	t.Helper()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(p)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
}

func TestNewTracer_EmitsSpanOnConfiguredProvider(t *testing.T) {
	ft := &fakeTracer{}
	withFakeProvider(t, &fakeProvider{tracer: ft})

	tracer := NewTracer()
	_, span := tracer.Start(context.Background(), "ai.run", Attr{Key: "ai.step_number", Value: 1})

	if ft.lastName != "ai.run" {
		t.Fatalf("span name = %q, want ai.run", ft.lastName)
	}
	if len(ft.lastSpan.attrs) != 1 || ft.lastSpan.attrs[0].Key != "ai.step_number" {
		t.Fatalf("attrs = %v, want one ai.step_number attribute", ft.lastSpan.attrs)
	}

	span.SetAttributes(Attr{Key: "ai.finish_reason", Value: "stop"})
	if len(ft.lastSpan.attrs) != 2 {
		t.Fatalf("expected SetAttributes to append, got %v", ft.lastSpan.attrs)
	}

	wantErr := errors.New("boom")
	span.RecordError(wantErr)
	if !errors.Is(ft.lastSpan.recordedErr, wantErr) {
		t.Fatalf("recordedErr = %v, want %v", ft.lastSpan.recordedErr, wantErr)
	}

	span.End()
	if !ft.lastSpan.ended {
		t.Fatal("expected End to reach the underlying span")
	}
}

func TestNewTracer_RecordErrorIgnoresNil(t *testing.T) {
	ft := &fakeTracer{}
	withFakeProvider(t, &fakeProvider{tracer: ft})

	_, span := NewTracer().Start(context.Background(), "ai.step")
	span.RecordError(nil)

	if ft.lastSpan.recordedErr != nil {
		t.Fatalf("RecordError(nil) must not reach the underlying span, got %v", ft.lastSpan.recordedErr)
	}
}

func TestNewTracer_DefaultProviderIsNoOp(t *testing.T) {
	// No provider registered in this test: NewTracer must still work without
	// panicking, and must not be mistaken for a real backend — this is the
	// "genuine no-op default" the OTel API itself guarantees.
	ctx, span := NewTracer().Start(context.Background(), "ai.run")
	if ctx == nil {
		t.Fatal("expected a non-nil context")
	}
	span.SetAttributes(Attr{Key: "k", Value: "v"})
	span.RecordError(errors.New("x"))
	span.End() // must not panic
}

func TestToKeyValues_ConvertsKnownTypes(t *testing.T) {
	kv := toKeyValues([]Attr{
		{Key: "s", Value: "text"},
		{Key: "i", Value: 7},
		{Key: "i64", Value: int64(8)},
		{Key: "b", Value: true},
		{Key: "other", Value: 3.14},
	})
	if len(kv) != 5 {
		t.Fatalf("len(kv) = %d, want 5", len(kv))
	}
	if kv[0].Value.AsString() != "text" {
		t.Errorf("string attr = %q", kv[0].Value.AsString())
	}
	if kv[1].Value.AsInt64() != 7 {
		t.Errorf("int attr = %v", kv[1].Value.AsInt64())
	}
	if kv[4].Value.AsString() != "3.14" {
		t.Errorf("fallback attr = %q, want fmt.Sprint fallback", kv[4].Value.AsString())
	}
}

func TestToKeyValues_EmptyIsNil(t *testing.T) {
	if kv := toKeyValues(nil); kv != nil {
		t.Fatalf("expected nil for no attrs, got %v", kv)
	}
}
