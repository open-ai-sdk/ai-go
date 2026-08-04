package agent

import (
	"context"
	"log/slog"
	"sync"
	"testing"
)

// recordingHandler captures every slog.Record it receives.
type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}
func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }
func (h *recordingHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.records)
}

// TestRunLoop_LoggerReceivesRecoveredPanic reuses the panicExecutor fixture
// from tool-loop-panic_test.go: a sequential tool-executor panic is a control
// callback, so safego.Recover surfaces it as a run-ending *PanicError. This
// test's addition is the Logger assertion — proving runConfig.Logger (set by
// ai.WithLogger at the public API layer) actually receives that event instead
// of the panic only reaching the returned error.
func TestRunLoop_LoggerReceivesRecoveredPanic(t *testing.T) {
	rec := &recordingHandler{}
	logger := slog.New(rec)

	model := &mockModel{calls: [][]StreamEvent{
		{toolCallEvt(0, "tc1", "boom", `{}`), finishEvt(FinishReasonToolCalls)},
	}}
	ch := driveStream(context.Background(), runConfig{
		Model:    model,
		Tools:    testToolSet(nil, panicExecutor{}),
		MaxSteps: 3,
		Logger:   logger,
	})
	for range ch {
	}

	if rec.count() == 0 {
		t.Fatal("expected the injected Logger to receive at least one record for the recovered panic")
	}
}
