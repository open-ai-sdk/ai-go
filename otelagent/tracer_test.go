package otelagent

import (
	"context"
	"errors"
	"testing"

	"github.com/open-ai-sdk/ai-go/agent"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

type recordedSpan struct {
	trace.Span
	name   string
	attrs  []attribute.KeyValue
	errors []error
	ended  bool
}

func (s *recordedSpan) End(...trace.SpanEndOption) { s.ended = true }

func (s *recordedSpan) SetAttributes(kv ...attribute.KeyValue) { s.attrs = append(s.attrs, kv...) }

func (s *recordedSpan) RecordError(err error, _ ...trace.EventOption) {
	s.errors = append(s.errors, err)
}

type recordingTracer struct {
	trace.Tracer
	spans []*recordedSpan
}

func (r *recordingTracer) Start(
	ctx context.Context,
	name string,
	opts ...trace.SpanStartOption,
) (context.Context, trace.Span) {
	span := &recordedSpan{Span: noop.Span{}, name: name}
	config := trace.NewSpanStartConfig(opts...)
	span.attrs = append(span.attrs, config.Attributes()...)
	r.spans = append(r.spans, span)
	return ctx, span
}

func lookup(attrs []attribute.KeyValue, key string) (attribute.Value, bool) {
	for _, kv := range attrs {
		if string(kv.Key) == key {
			return kv.Value, true
		}
	}
	return attribute.Value{}, false
}

func TestAdapterMapsGenAIAttributeTypes(t *testing.T) {
	recorder := &recordingTracer{Tracer: noop.NewTracerProvider().Tracer("test")}
	tracer := New(recorder)

	_, span := tracer.Start(context.Background(), "ai.model_call",
		agent.Attr{Key: "gen_ai.request.model", Value: "model-a"},
		agent.Attr{Key: "ai.step_number", Value: 3},
	)
	span.SetAttributes(
		agent.Attr{Key: "gen_ai.response.id", Value: "resp_1"},
		agent.Attr{Key: "gen_ai.usage.input_tokens", Value: 10},
		agent.Attr{Key: "gen_ai.usage.big", Value: int64(1 << 40)},
		agent.Attr{Key: "gen_ai.response.finish_reasons", Value: []string{"stop", "length"}},
		agent.Attr{Key: "ratio", Value: 0.5},
		agent.Attr{Key: "flag", Value: true},
		agent.Attr{Key: "other", Value: struct{ A int }{A: 1}},
	)
	span.RecordError(errors.New("boom"))
	span.RecordError(nil)
	span.End()

	if len(recorder.spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(recorder.spans))
	}
	got := recorder.spans[0]
	if got.name != "ai.model_call" || !got.ended || len(got.errors) != 1 {
		t.Fatalf("span = %+v", got)
	}
	checks := []struct {
		key  string
		test func(attribute.Value) bool
	}{
		{"gen_ai.request.model", func(v attribute.Value) bool { return v.AsString() == "model-a" }},
		{"ai.step_number", func(v attribute.Value) bool { return v.AsInt64() == 3 }},
		{"gen_ai.response.id", func(v attribute.Value) bool { return v.AsString() == "resp_1" }},
		{"gen_ai.usage.input_tokens", func(v attribute.Value) bool { return v.AsInt64() == 10 }},
		{"gen_ai.usage.big", func(v attribute.Value) bool { return v.AsInt64() == 1<<40 }},
		{"gen_ai.response.finish_reasons", func(v attribute.Value) bool {
			s := v.AsStringSlice()
			return v.Type() == attribute.STRINGSLICE && len(s) == 2 && s[0] == "stop" && s[1] == "length"
		}},
		{"ratio", func(v attribute.Value) bool { return v.Type() == attribute.FLOAT64 && v.AsFloat64() == 0.5 }},
		{"flag", func(v attribute.Value) bool { return v.AsBool() }},
		{"other", func(v attribute.Value) bool { return v.AsString() == "{1}" }},
	}
	for _, check := range checks {
		value, ok := lookup(got.attrs, check.key)
		if !ok || !check.test(value) {
			t.Errorf("attribute %q = %v (present=%v)", check.key, value, ok)
		}
	}
}

func TestKeyValuesEmptyIsNil(t *testing.T) {
	if got := keyValues(nil); got != nil {
		t.Fatalf("keyValues(nil) = %v", got)
	}
}
