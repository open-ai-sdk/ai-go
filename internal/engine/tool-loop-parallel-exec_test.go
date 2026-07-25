package engine

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// fixedToolCallsModel emits a fixed batch of tool-call deltas (one per event,
// each its own ToolCallIndex) in a single step, then finishes with
// FinishReasonToolCalls.
type fixedToolCallsModel struct{ toolNames []string }

func (fixedToolCallsModel) ModelID() string { return "fixed-tool-calls" }

func (m fixedToolCallsModel) Stream(context.Context, Request) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, len(m.toolNames)+1)
	for i, name := range m.toolNames {
		ch <- StreamEvent{
			Type:              StreamEventToolCallDelta,
			ToolCallIndex:     i,
			ToolCallID:        fmt.Sprintf("tc-%d", i),
			ToolCallName:      name,
			ToolCallArgsDelta: "{}",
		}
	}
	ch <- StreamEvent{Type: StreamEventFinish, FinishReason: FinishReasonToolCalls}
	close(ch)
	return ch, nil
}

// siblingFailureExecutor fails exactly one tool name and succeeds on the rest,
// so a run can prove one failing call does not take its siblings down with it.
type siblingFailureExecutor struct{ failName string }

func (e siblingFailureExecutor) Execute(_ context.Context, name, _ string) (string, error) {
	if name == e.failName {
		return "", errors.New("boom")
	}
	return fmt.Sprintf(`{"tool":%q}`, name), nil
}

// TestExecuteToolCallsParallel_SiblingFailure_OthersComplete proves that
// errgroup's concurrency limiting is not paired with its fail-fast error
// aggregation: one tool call erroring must not cancel or drop its siblings'
// results, matching node's per-call error reporting.
func TestExecuteToolCallsParallel_SiblingFailure_OthersComplete(t *testing.T) {
	names := []string{"ok-1", "fail", "ok-2"}
	ch := Run(context.Background(), RunParams{
		Model: fixedToolCallsModel{toolNames: names},
		Tools: &ToolSet{
			Definitions: []ToolDefinition{{Name: "ok-1"}, {Name: "fail"}, {Name: "ok-2"}},
			Executor:    siblingFailureExecutor{failName: "fail"},
		},
		ParallelToolExecution: true,
		MaxParallelTools:      3,
		MaxSteps:              1,
	})

	results := make(map[string]ToolResult, 3)
	for ev := range ch {
		if ev.Type == StepEventToolResult && ev.ToolResult != nil {
			results[ev.ToolResult.Name] = *ev.ToolResult
		}
	}

	if len(results) != 3 {
		t.Fatalf("got %d tool results, want 3 (a sibling failure must not drop the others)", len(results))
	}
	if results["ok-1"].Output != `{"tool":"ok-1"}` {
		t.Errorf(`ok-1 output = %q, want {"tool":"ok-1"}`, results["ok-1"].Output)
	}
	if results["ok-2"].Output != `{"tool":"ok-2"}` {
		t.Errorf(`ok-2 output = %q, want {"tool":"ok-2"}`, results["ok-2"].Output)
	}
	if results["fail"].Output != `{"error":"boom"}` {
		t.Errorf(`fail output = %q, want {"error":"boom"}`, results["fail"].Output)
	}
}

// queueGateExecutor lets a test observe exactly when a tool call's body
// starts, then hold it open until told to release — used to deterministically
// land a cancellation while sibling calls are still queued behind a
// concurrency limit of 1.
type queueGateExecutor struct {
	started  chan struct{}
	release  chan struct{}
	bodyRuns int32
}

func (e *queueGateExecutor) Execute(context.Context, string, string) (string, error) {
	atomic.AddInt32(&e.bodyRuns, 1)
	e.started <- struct{}{}
	<-e.release
	return `{"ok":true}`, nil
}

// TestExecuteToolCallsParallel_CancelWhileQueued_StopsFurtherBodies proves
// that errgroup.SetLimit's queueing happens before a call's goroutine starts:
// with a concurrency limit of 1 and 10 pending calls, cancelling the run's
// context while the first call is in flight must stop every call still
// queued behind it from ever invoking the tool executor.
func TestExecuteToolCallsParallel_CancelWhileQueued_StopsFurtherBodies(t *testing.T) {
	const numCalls = 10
	names := make([]string, numCalls)
	for i := range names {
		names[i] = "slow"
	}

	executor := &queueGateExecutor{started: make(chan struct{}), release: make(chan struct{})}

	ctx, cancel := context.WithCancel(context.Background())
	ch := Run(ctx, RunParams{
		Model: fixedToolCallsModel{toolNames: names},
		Tools: &ToolSet{
			Definitions: []ToolDefinition{{Name: "slow"}},
			Executor:    executor,
		},
		ParallelToolExecution: true,
		MaxParallelTools:      1,
		MaxSteps:              1,
	})

	// Drain the run's event channel concurrently so its ctx-guarded emit()
	// never blocks on an unread channel while the test holds the first call open.
	drained := make(chan struct{})
	go func() {
		for range ch {
		}
		close(drained)
	}()

	select {
	case <-executor.started:
	case <-time.After(2 * time.Second):
		t.Fatal("first tool call never started")
	}

	cancel()                // cancel while calls 2-10 are queued behind the limit-1 semaphore
	close(executor.release) // let call 1 finish; calls 2-10 must see the run already cancelled

	select {
	case <-drained:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not complete after cancellation")
	}

	if got := atomic.LoadInt32(&executor.bodyRuns); got != 1 {
		t.Errorf(
			"executor.bodyRuns = %d, want exactly 1 (queued calls must not run their body after cancellation)",
			got,
		)
	}
}
