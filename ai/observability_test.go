package ai_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
)

// recordingSlogHandler captures every slog.Record it receives, so a test can
// assert exactly what — if anything — was logged.
type recordingSlogHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingSlogHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordingSlogHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}
func (h *recordingSlogHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingSlogHandler) WithGroup(string) slog.Handler      { return h }
func (h *recordingSlogHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.records)
}

// panickingToolExecutor panics on every Execute call, exercising the
// recovery boundary at internal/safego that ai.WithLogger is meant to reach.
type panickingToolExecutor struct{}

func (panickingToolExecutor) Execute(context.Context, string, string) (string, error) {
	panic("boom from a consumer's tool executor")
}

// singleToolCallModel emits exactly one tool call, then finishes — enough to
// drive the panic path deterministically in one step.
type singleToolCallModel struct{}

func (singleToolCallModel) ModelID() string { return "single-tool-call" }

func (singleToolCallModel) Stream(context.Context, ai.LanguageModelRequest) (<-chan ai.StreamEvent, error) {
	ch := make(chan ai.StreamEvent, 2)
	ch <- ai.StreamEvent{Type: ai.StreamEventToolCallDelta, ToolCallID: "tc1", ToolCallName: "boom", ToolCallArgsDelta: `{}`}
	ch <- ai.StreamEvent{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonToolCalls}
	close(ch)
	return ch, nil
}

// TestWithLogger_ReceivesRecoveredPanic proves ai.WithLogger threads through
// to the engine's panic-recovery boundary (internal/safego) end to end: a
// tool executor panic — a control callback, per safego's own doc comment —
// is recovered, fails the run with a *PanicError, and is also logged to the
// caller-supplied logger, not just returned as an error value.
func TestWithLogger_ReceivesRecoveredPanic(t *testing.T) {
	rec := &recordingSlogHandler{}
	logger := slog.New(rec)

	rt := ai.NewRuntime(ai.WithDefaultModel(singleToolCallModel{}))
	_, err := rt.GenerateText(context.Background(), "go",
		ai.WithTools(&ai.ToolSet{
			Definitions: []ai.ToolDefinition{{Name: "boom", InputSchema: map[string]any{"type": "object"}}},
			Executor:    panickingToolExecutor{},
		}),
		ai.WithLogger(logger),
	)
	if err == nil {
		t.Fatal("expected the panicking tool call to fail the run")
	}
	if rec.count() == 0 {
		t.Fatal("expected ai.WithLogger's logger to receive at least one record for the recovered panic")
	}
}

// TestNoLoggerConfigured_NeverWritesToSlogDefault is the success criterion
// from the observability requirements made concrete: with no WithLogger
// option set, the SDK must produce zero log lines — in particular it must
// never fall back to slog.Default(), which would start writing into a
// consumer's log stream without being asked. The same panic scenario as
// above is used so this test would fail loudly if that fallback ever crept
// back in.
func TestNoLoggerConfigured_NeverWritesToSlogDefault(t *testing.T) {
	rec := &recordingSlogHandler{}
	orig := slog.Default()
	slog.SetDefault(slog.New(rec))
	defer slog.SetDefault(orig)

	rt := ai.NewRuntime(ai.WithDefaultModel(singleToolCallModel{}))
	_, _ = rt.GenerateText(context.Background(), "go",
		ai.WithTools(&ai.ToolSet{
			Definitions: []ai.ToolDefinition{{Name: "boom", InputSchema: map[string]any{"type": "object"}}},
			Executor:    panickingToolExecutor{},
		}),
	)

	if got := rec.count(); got != 0 {
		t.Fatalf("expected zero records on slog.Default() with no WithLogger, got %d", got)
	}
}

// TestNoOptionsConfigured_RunsCleanlyWithDefaultTracing is a smoke test for
// the tracing side of the same guarantee: with no otel.TracerProvider
// registered (the state of the process by default, and restored by every
// other test in this suite via t.Cleanup), a normal call must complete
// without error. The no-op guarantee itself — that tracing.NewTracer's
// default costs nothing observable — is asserted precisely in
// internal/tracing; this test only proves the ai package's wiring into it
// doesn't break the ordinary path.
func TestNoOptionsConfigured_RunsCleanlyWithDefaultTracing(t *testing.T) {
	result, err := ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model: &stubLanguageModel{
			events: []ai.StreamEvent{
				{Type: ai.StreamEventTextDelta, TextDelta: "hi"},
				{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonStop},
			},
		},
		Messages: []ai.Message{ai.UserMessage("go")},
	})
	if err != nil {
		t.Fatalf("unexpected error with zero-config observability: %v", err)
	}
	if result.Text != "hi" {
		t.Fatalf("Text = %q, want %q", result.Text, "hi")
	}
}
