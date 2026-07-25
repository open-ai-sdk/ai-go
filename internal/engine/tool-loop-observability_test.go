package engine

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/open-ai-sdk/ai-go/internal/tracing"
)

// recordedSpan captures what one Start/End/SetAttributes/RecordError sequence
// recorded, so a test can inspect span names, attributes, and error state
// after a run completes.
type recordedSpan struct {
	name  string
	attrs []tracing.Attr
	ended bool
	err   error
}

// fakeTracer is a tracing.Tracer that records every span it starts. Guarded
// by a mutex because parallel tool execution starts tool-call spans from
// multiple goroutines concurrently.
type fakeTracer struct {
	mu    sync.Mutex
	spans []*recordedSpan
}

func (f *fakeTracer) Start(ctx context.Context, name string, attrs ...tracing.Attr) (context.Context, tracing.Span) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := &recordedSpan{name: name, attrs: append([]tracing.Attr{}, attrs...)}
	f.spans = append(f.spans, s)
	return ctx, &fakeSpan{s: s, mu: &f.mu}
}

func (f *fakeTracer) byName(name string) []*recordedSpan {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*recordedSpan
	for _, s := range f.spans {
		if s.name == name {
			out = append(out, s)
		}
	}
	return out
}

// attrValues collects every attribute value recorded across all spans, as
// strings, so a test can grep for content that must (or must not) appear.
func (f *fakeTracer) attrValues() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, s := range f.spans {
		for _, a := range s.attrs {
			if str, ok := a.Value.(string); ok {
				out = append(out, str)
			}
		}
	}
	return out
}

type fakeSpan struct {
	s  *recordedSpan
	mu *sync.Mutex
}

func (r *fakeSpan) End() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.s.ended = true
}

func (r *fakeSpan) SetAttributes(attrs ...tracing.Attr) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.s.attrs = append(r.s.attrs, attrs...)
}

func (r *fakeSpan) RecordError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.s.err = err
}

// runObservabilityScenario drives a two-step tool loop (one tool call, one
// final text step) so every span kind (run, step, model call, tool call) is
// exercised exactly once, with a distinctive marker in the prompt, the tool
// arguments, and the tool output — so tests can assert whether that marker
// leaked into span attributes.
func runObservabilityScenario(t *testing.T, tracer tracing.Tracer, logger *slog.Logger, traceContent bool) *fakeTracer {
	t.Helper()
	model := &mockModel{calls: [][]StreamEvent{
		{
			toolCallEvt(0, "tc1", "lookup", `{"q":"PROMPT_MARKER"}`),
			{Type: StreamEventUsage, Usage: &Usage{InputTokens: 10, OutputTokens: 2, TotalTokens: 12}},
			finishEvt(FinishReasonToolCalls),
		},
		{
			textEvt("done"),
			{Type: StreamEventUsage, Usage: &Usage{InputTokens: 5, OutputTokens: 1, TotalTokens: 6}},
			finishEvt(FinishReasonStop),
		},
	}}
	exec := &mockExecutor{results: map[string]string{"lookup": `{"result":"TOOL_OUTPUT_MARKER"}`}}

	ch := Run(context.Background(), RunParams{
		Model: model,
		Tools: &ToolSet{Executor: exec},
		Request: Request{
			Messages: []Message{{Role: "user", Content: []ContentPart{{Type: "text", Text: "PROMPT_MARKER"}}}},
		},
		MaxSteps:     5,
		Logger:       logger,
		Tracer:       tracer,
		TraceContent: traceContent,
	})
	for ev := range ch {
		if ev.Type == StepEventError {
			t.Fatalf("unexpected error: %v", ev.Error)
		}
	}

	ft, ok := tracer.(*fakeTracer)
	if !ok {
		t.Fatalf("test scenario requires a *fakeTracer, got %T", tracer)
	}
	return ft
}

func TestRunLoop_EmitsRunStepModelCallToolCallSpans(t *testing.T) {
	ft := runObservabilityScenario(t, &fakeTracer{}, nil, false)

	if got := ft.byName("ai.run"); len(got) != 1 {
		t.Fatalf("ai.run spans = %d, want 1", len(got))
	}
	if got := ft.byName("ai.step"); len(got) != 2 {
		t.Fatalf("ai.step spans = %d, want 2 (tool-call step + final step)", len(got))
	}
	if got := ft.byName("ai.model_call"); len(got) != 2 {
		t.Fatalf("ai.model_call spans = %d, want 2", len(got))
	}
	if got := ft.byName("ai.tool_call"); len(got) != 1 {
		t.Fatalf("ai.tool_call spans = %d, want 1", len(got))
	}

	for _, name := range []string{"ai.run", "ai.step", "ai.model_call", "ai.tool_call"} {
		for _, s := range ft.byName(name) {
			if !s.ended {
				t.Errorf("span %q was never ended", name)
			}
		}
	}
}

func TestRunLoop_SpanAttributes_ModelStepAndToolMetadata(t *testing.T) {
	ft := runObservabilityScenario(t, &fakeTracer{}, nil, false)

	toolSpans := ft.byName("ai.tool_call")
	if len(toolSpans) != 1 {
		t.Fatalf("expected 1 tool span, got %d", len(toolSpans))
	}
	if !hasAttr(toolSpans[0].attrs, "ai.tool_name", "lookup") {
		t.Errorf("tool span attrs = %v, want ai.tool_name=lookup", toolSpans[0].attrs)
	}

	stepSpans := ft.byName("ai.step")
	for i, s := range stepSpans {
		if !hasAttrKey(s.attrs, "ai.step_number") {
			t.Errorf("step span %d missing ai.step_number: %v", i, s.attrs)
		}
		if !hasAttrKey(s.attrs, "ai.model_id") {
			t.Errorf("step span %d missing ai.model_id: %v", i, s.attrs)
		}
		if !hasAttrKey(s.attrs, "ai.finish_reason") {
			t.Errorf("step span %d missing ai.finish_reason: %v", i, s.attrs)
		}
		if !hasAttrKey(s.attrs, "ai.usage.total_tokens") {
			t.Errorf("step span %d missing ai.usage.total_tokens: %v", i, s.attrs)
		}
	}
}

func TestRunLoop_SpansExcludeContentByDefault(t *testing.T) {
	ft := runObservabilityScenario(t, &fakeTracer{}, nil, false)

	for _, v := range ft.attrValues() {
		if strings.Contains(v, "PROMPT_MARKER") || strings.Contains(v, "TOOL_OUTPUT_MARKER") {
			t.Fatalf("span attribute leaked content by default: %q", v)
		}
	}
}

func TestRunLoop_SpansIncludeContentWhenTraceContentEnabled(t *testing.T) {
	ft := runObservabilityScenario(t, &fakeTracer{}, nil, true)

	values := ft.attrValues()
	var sawPrompt, sawToolArgs, sawToolOutput bool
	for _, v := range values {
		if strings.Contains(v, "PROMPT_MARKER") {
			sawPrompt = true
		}
		if strings.Contains(v, `"q":"PROMPT_MARKER"`) {
			sawToolArgs = true
		}
		if strings.Contains(v, "TOOL_OUTPUT_MARKER") {
			sawToolOutput = true
		}
	}
	if !sawPrompt {
		t.Error("expected a prompt-content attribute when WithTraceContent(true)")
	}
	if !sawToolArgs {
		t.Error("expected a tool-arguments attribute when WithTraceContent(true)")
	}
	if !sawToolOutput {
		t.Error("expected a tool-output attribute when WithTraceContent(true)")
	}
}

func TestRunLoop_NilTracer_DoesNotPanic(t *testing.T) {
	model := &mockModel{calls: [][]StreamEvent{{textEvt("hi"), finishEvt(FinishReasonStop)}}}
	ch := Run(context.Background(), RunParams{Model: model, MaxSteps: 1})
	for ev := range ch {
		if ev.Type == StepEventError {
			t.Fatalf("unexpected error: %v", ev.Error)
		}
	}
}

func hasAttrKey(attrs []tracing.Attr, key string) bool {
	for _, a := range attrs {
		if a.Key == key {
			return true
		}
	}
	return false
}

func hasAttr(attrs []tracing.Attr, key string, value any) bool {
	for _, a := range attrs {
		if a.Key == key && a.Value == value {
			return true
		}
	}
	return false
}
