package ai

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// feedAsync starts a goroutine that sends events on src after the caller has had
// a chance to register its views, then closes src. Registering views before the
// first event guarantees every view sees the full sequence.
func feedAsync(src chan<- StepEvent, events ...StepEvent) {
	go func() {
		for _, e := range events {
			src <- e
		}
		close(src)
	}()
}

var viewEvents = []StepEvent{
	{Type: StepEventStepStart},
	{Type: StepEventTextDelta, TextDelta: "a"},
	{Type: StepEventTextDelta, TextDelta: "b"},
	{Type: StepEventStepEnd, FinishReason: FinishReasonStop},
}

// branchCount reports how many fan-out branches are currently registered.
// Used by tests to barrier on registration before feeding events, since Events()
// is a lazy iterator that only registers its branch when the range starts.
func (sr *StreamResult) branchCount() int {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	return len(sr.branches)
}

// TestStreamResult_ConcurrentViews exercises Stream, TextStream, and Events on
// one result at once; each must receive the full sequence. Run with -race.
func TestStreamResult_ConcurrentViews(t *testing.T) {
	src := make(chan StepEvent)
	sr := NewStreamResult(src)

	// Stream and TextStream register eagerly.
	stream := sr.Stream()
	text := sr.TextStream()

	var (
		wg          sync.WaitGroup
		streamCount int
		textCount   int
		eventsCount int
	)
	wg.Add(3)
	go func() {
		defer wg.Done()
		for range stream {
			streamCount++
		}
	}()
	go func() {
		defer wg.Done()
		for range text {
			textCount++
		}
	}()
	go func() {
		defer wg.Done()
		// Ranging registers the Events branch synchronously before the first yield.
		for _, err := range sr.Events() {
			if err == nil {
				eventsCount++
			}
		}
	}()

	// Barrier: wait until all three branches are registered before feeding, so
	// every view sees the full sequence (a late view legitimately sees less).
	deadline := time.Now().Add(2 * time.Second)
	for sr.branchCount() < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("only %d/3 branches registered", sr.branchCount())
		}
		time.Sleep(time.Millisecond)
	}
	feedAsync(src, viewEvents...)
	wg.Wait()

	if streamCount != len(viewEvents) {
		t.Errorf("Stream view saw %d events, want %d", streamCount, len(viewEvents))
	}
	if eventsCount != len(viewEvents) {
		t.Errorf("Events view saw %d events, want %d", eventsCount, len(viewEvents))
	}
	if textCount != 2 {
		t.Errorf("TextStream saw %d deltas, want 2", textCount)
	}
	select {
	case <-sr.done:
	case <-time.After(2 * time.Second):
		t.Fatal("fan-out goroutine did not finish")
	}
}

// TestStreamResult_LateViewGetsNamedError verifies a view registered after the
// source is fully consumed receives ErrStreamConsumed, never a silent close.
func TestStreamResult_LateViewGetsNamedError(t *testing.T) {
	src := make(chan StepEvent, len(viewEvents))
	for _, e := range viewEvents {
		src <- e
	}
	close(src)
	sr := NewStreamResult(src)

	if _, err := sr.Consume(); err != nil {
		t.Fatalf("first Consume: %v", err)
	}

	// Source is exhausted; a late Stream() must surface an error event.
	ch := sr.Stream()
	var sawErr error
	for ev := range ch {
		if ev.Type == StepEventError {
			sawErr = ev.Error
		}
	}
	if !errors.Is(sawErr, ErrStreamConsumed) {
		t.Fatalf("late Stream() error = %v, want ErrStreamConsumed", sawErr)
	}

	// Events() must yield the same named error.
	var iterErr error
	for _, err := range sr.Events() {
		if err != nil {
			iterErr = err
		}
	}
	if !errors.Is(iterErr, ErrStreamConsumed) {
		t.Fatalf("late Events() error = %v, want ErrStreamConsumed", iterErr)
	}
}

// TestStreamResult_EventsBreakDoesNotTearDownOthers verifies that breaking an
// Events() range unregisters only that branch: a concurrent Stream() view still
// receives every event and the run completes.
func TestStreamResult_EventsBreakDoesNotTearDownOthers(t *testing.T) {
	src := make(chan StepEvent)
	sr := NewStreamResult(src)

	stream := sr.Stream()
	events := sr.Events()

	feedAsync(src, viewEvents...)

	// Break Events after the first event.
	brokeAfter := 0
	go func() {
		for range events {
			brokeAfter++
			break
		}
	}()

	streamCount := 0
	for range stream {
		streamCount++
	}

	if streamCount != len(viewEvents) {
		t.Errorf("Stream view saw %d events after Events broke, want %d", streamCount, len(viewEvents))
	}
	select {
	case <-sr.done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not complete after a view broke")
	}
}
