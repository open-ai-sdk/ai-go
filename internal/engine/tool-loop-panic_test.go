package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/open-ai-sdk/ai-go/internal/safego"
)

type panicExecutor struct{}

func (panicExecutor) Execute(_ context.Context, _, _ string) (string, error) {
	panic("tool executor boom")
}

// TestRun_ToolExecutorPanic_FailsRunWithPanicError verifies that a panic in a
// tool executor (a control callback) is recovered, surfaced as a *PanicError on
// the stream before the channel closes, and does not crash the process.
func TestRun_ToolExecutorPanic_FailsRunWithPanicError(t *testing.T) {
	model := &mockModel{calls: [][]StreamEvent{
		{toolCallEvt(0, "tc1", "boom", `{}`), finishEvt(FinishReasonToolCalls)},
		{textEvt("after"), finishEvt(FinishReasonStop)},
	}}

	ch := Run(context.Background(), RunParams{
		Model:    model,
		Tools:    &ToolSet{Executor: panicExecutor{}},
		MaxSteps: 3,
	})

	var lastErr error
	sawError := false
	for ev := range ch {
		if ev.Type == StepEventError {
			sawError = true
			lastErr = ev.Error
		}
	}

	if !sawError {
		t.Fatal("expected a StepEventError from the panicking executor")
	}
	var pe *safego.PanicError
	if !errors.As(lastErr, &pe) {
		t.Fatalf("expected *safego.PanicError, got %T (%v)", lastErr, lastErr)
	}
	if len(pe.Stack) == 0 {
		t.Error("expected a non-empty stack trace on the PanicError")
	}
}

// TestRun_OnChunkPanic_RunCompletes verifies that a panic in an observer
// callback (OnChunk) is recovered and swallowed: the run completes normally and
// no error is surfaced (node mergeCallbacks parity).
func TestRun_OnChunkPanic_RunCompletes(t *testing.T) {
	model := &mockModel{calls: [][]StreamEvent{
		{textEvt("hello"), finishEvt(FinishReasonStop)},
	}}
	cb := &LifecycleCallbacks{OnChunk: func(StepEvent) { panic("observer boom") }}

	ch := Run(context.Background(), RunParams{Model: model, MaxSteps: 1, Callbacks: cb})

	sawDone, sawError := false, false
	for ev := range ch {
		switch ev.Type {
		case StepEventDone:
			sawDone = true
		case StepEventError:
			sawError = true
		}
	}

	if sawError {
		t.Error("an observer-callback panic must not surface as a run error")
	}
	if !sawDone {
		t.Error("run should complete normally despite an observer panic")
	}
}
