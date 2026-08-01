package generate

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestMergeCallback_BothNilReturnsNil verifies no wrapper is allocated when
// neither side is set, so an unused Agent callback slot costs nothing.
func TestMergeCallback_BothNilReturnsNil(t *testing.T) {
	merged := mergeCallback[int](nil, nil)
	if merged != nil {
		t.Fatal("expected nil merged callback when both sides are nil")
	}
}

// TestMergeCallback_OnlyAgentSet verifies the agent callback is returned
// unchanged (no goroutine indirection) when there is nothing to merge with.
func TestMergeCallback_OnlyAgentSet(t *testing.T) {
	var got int
	agent := func(v int) { got = v }
	merged := mergeCallback(agent, nil)
	merged(7)
	if got != 7 {
		t.Fatalf("got = %d, want 7", got)
	}
}

// TestMergeCallback_OnlyCallSet mirrors TestMergeCallback_OnlyAgentSet for the
// call-level side.
func TestMergeCallback_OnlyCallSet(t *testing.T) {
	var got int
	call := func(v int) { got = v }
	merged := mergeCallback(nil, call)
	merged(9)
	if got != 9 {
		t.Fatalf("got = %d, want 9", got)
	}
}

// TestMergeCallback_BothFire proves agent-level and call-level callbacks both
// run for the same event rather than a sequential "agent's callback wins"
// implementation.
func TestMergeCallback_BothFire(t *testing.T) {
	var agentFired, callFired int32
	var wg sync.WaitGroup
	wg.Add(2)
	agent := func(int) { atomic.StoreInt32(&agentFired, 1); wg.Done() }
	call := func(int) { atomic.StoreInt32(&callFired, 1); wg.Done() }

	mergeCallback(agent, call)(1)
	wg.Wait()

	if atomic.LoadInt32(&agentFired) != 1 {
		t.Error("expected agent-level callback to fire")
	}
	if atomic.LoadInt32(&callFired) != 1 {
		t.Error("expected call-level callback to fire")
	}
}

// TestMergeCallback_PanicInOneDoesNotStopTheOther proves a panicking callback
// is swallowed (recovered and logged, not propagated) and does not prevent
// the other callback from running or fail the merged call itself.
func TestMergeCallback_PanicInOneDoesNotStopTheOther(t *testing.T) {
	var callFired int32
	agent := func(int) { panic("boom") }
	call := func(int) { atomic.StoreInt32(&callFired, 1) }

	merged := mergeCallback(agent, call)
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("merged callback must not propagate a panic, got: %v", r)
			}
		}()
		merged(1)
	}()

	if atomic.LoadInt32(&callFired) != 1 {
		t.Error("expected call-level callback to still fire despite the agent-level panic")
	}
}

func TestMergeCallback_RunsBothCallbacksConcurrently(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	callback := func(int) {
		started <- struct{}{}
		<-release
	}
	done := make(chan struct{})
	go func() {
		mergeCallback(callback, callback)(1)
		close(done)
	}()

	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("callbacks did not start concurrently")
		}
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("merged callback did not settle after both callbacks returned")
	}
}
