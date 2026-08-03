package agent

import (
	"context"
	"log/slog"
	"testing"

	"github.com/open-ai-sdk/ai-go/internal/tracing"
)

// benchModel is a stateless single-step model safe to reuse across
// b.N iterations (unlike mockModel, which consumes one pre-canned call per
// invocation and would run out partway through a benchmark).
type benchModel struct{}

func (benchModel) ModelID() string { return "bench" }

func (benchModel) Stream(context.Context, Request) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 3)
	ch <- StreamEvent{Type: StreamEventTextDelta, TextDelta: "hello world"}
	ch <- StreamEvent{Type: StreamEventUsage, Usage: &Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}}
	ch <- StreamEvent{Type: StreamEventFinish, FinishReason: FinishReasonStop}
	close(ch)
	return ch, nil
}

func runBenchOnce(params runConfig) {
	ch := driveStream(context.Background(), params)
	for range ch {
	}
}

// BenchmarkToolLoop_InstrumentationDisabled is the default a caller gets with
// no instrumentation configured. The core's default is a true no-op.
func BenchmarkToolLoop_InstrumentationDisabled(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		runBenchOnce(runConfig{Model: benchModel{}, MaxSteps: 1})
	}
}

// BenchmarkToolLoop_InstrumentationEnabled measures an explicitly injected
// tracer plus a discard-handler slog.Logger.
func BenchmarkToolLoop_InstrumentationEnabled(b *testing.B) {
	b.ReportAllocs()
	logger := slog.New(slog.DiscardHandler)
	tracer := tracing.NewTracer()
	for i := 0; i < b.N; i++ {
		runBenchOnce(runConfig{Model: benchModel{}, MaxSteps: 1, Logger: logger, Tracer: tracer})
	}
}
