package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// ctxAwareModel streams text deltas until its context is cancelled, then records
// that it observed the cancellation (a stand-in for closing the HTTP body) and
// closes its channel. Every send selects on ctx.Done so it never parks.
type ctxAwareModel struct {
	observedCancel int32
}

func (m *ctxAwareModel) ModelID() string { return "ctx-aware" }

func (m *ctxAwareModel) Stream(ctx context.Context, _ Request) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent)
	go func() {
		defer close(ch)
		for {
			select {
			case ch <- StreamEvent{Type: StreamEventTextDelta, TextDelta: "x"}:
			case <-ctx.Done():
				atomic.StoreInt32(&m.observedCancel, 1)
				return
			}
		}
	}()
	return ch, nil
}

type delayedStreamModel struct{}

func (delayedStreamModel) ModelID() string { return "delayed-stream" }

func (delayedStreamModel) Stream(ctx context.Context, _ Request) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent)
	go func() {
		defer close(ch)
		time.Sleep(10 * time.Millisecond)
		for _, event := range []StreamEvent{
			{Type: StreamEventTextDelta, TextDelta: "delayed"},
			{Type: StreamEventFinish, FinishReason: FinishReasonStop},
		} {
			select {
			case ch <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// hungProviderModel represents a provider that releases its transport when
// its context is cancelled but, due to a decoder bug, never closes the event
// channel. The agent must not range that channel forever.
type hungProviderModel struct {
	providerDone chan struct{}
}

func (m *hungProviderModel) ModelID() string { return "hung-provider" }

func (m *hungProviderModel) Stream(ctx context.Context, _ Request) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent)
	go func() {
		<-ctx.Done()
		close(m.providerDone)
		// Intentionally leave ch open: the agent owns cancellation of its own
		// output, not correctness of a third-party provider channel.
	}()
	return ch, nil
}

type errorThenOpenModel struct {
	providerDone chan struct{}
}

func (m *errorThenOpenModel) ModelID() string { return "error-then-open" }

func (m *errorThenOpenModel) Stream(ctx context.Context, _ Request) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 1)
	ch <- StreamEvent{Type: StreamEventError, Error: errors.New("provider failed")}
	go func() {
		<-ctx.Done()
		close(m.providerDone)
	}()
	return ch, nil
}

// TestRun_DoesNotCancelBeforeAsyncStreamIsConsumed protects providers that
// return their event channel before the first network event is available.
func TestRun_DoesNotCancelBeforeAsyncStreamIsConsumed(t *testing.T) {
	ch := Run(context.Background(), RunParams{
		Model:    delayedStreamModel{},
		MaxSteps: 1,
	})

	var text string
	for event := range ch {
		if event.Type == StepEventTextDelta {
			text += event.TextDelta
		}
	}
	if text != "delayed" {
		t.Fatalf("streamed text = %q, want delayed", text)
	}
}

// TestRun_CancelMidStream_ClosesChannelAndReleasesProvider verifies that
// cancelling the run's context mid-stream promptly closes the output channel and
// the provider stream goroutine observes the cancellation (releases its body).
func TestRun_CancelMidStream_ClosesChannelAndReleasesProvider(t *testing.T) {
	model := &ctxAwareModel{}
	ctx, cancel := context.WithCancel(context.Background())

	ch := Run(ctx, RunParams{Model: model, MaxSteps: 1})

	// Consume a few events, then abandon by cancelling.
	<-ch // StepStart
	<-ch // a text delta
	cancel()

	// The output channel must close promptly once cancelled.
	closed := make(chan struct{})
	go func() {
		for range ch {
		}
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Run channel did not close after context cancellation")
	}

	// The provider goroutine must have observed the cancellation (no leak).
	deadline := time.Now().Add(time.Second)
	for atomic.LoadInt32(&model.observedCancel) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("provider stream goroutine did not observe cancellation")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestRun_CancelBeforeStart_ReportsCancellation verifies that a context
// cancelled before Run starts terminates at the first step boundary and still
// reports why it stopped.
func TestRun_CancelBeforeStart_ReportsCancellation(t *testing.T) {
	model := &ctxAwareModel{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	ch := Run(ctx, RunParams{Model: model, MaxSteps: 3})

	var gotErr error
	for event := range ch {
		if event.Type == StepEventError {
			gotErr = event.Error
		}
	}
	if !errors.Is(gotErr, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", gotErr)
	}
}

func TestRun_CancelHungProvider_ClosesOutputAndReleasesProvider(t *testing.T) {
	model := &hungProviderModel{providerDone: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	ch := Run(ctx, RunParams{Model: model})

	if event := <-ch; event.Type != StepEventStepStart {
		t.Fatalf("first event = %v, want StepEventStepStart", event.Type)
	}
	cancel()

	select {
	case <-model.providerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("hung provider did not observe cancellation")
	}
	var gotErr error
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for event := range ch {
			if event.Type == StepEventError {
				gotErr = event.Error
			}
		}
	}()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("agent output did not close after cancelling a hung provider")
	}
	if !errors.Is(gotErr, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", gotErr)
	}
}

func TestRun_ProviderErrorThenOpenChannel_DoesNotLeakDrain(t *testing.T) {
	model := &errorThenOpenModel{providerDone: make(chan struct{})}
	ch := Run(context.Background(), RunParams{Model: model})

	var sawError bool
	for event := range ch {
		if event.Type == StepEventError {
			sawError = true
		}
	}
	if !sawError {
		t.Fatal("provider error was not emitted")
	}
	select {
	case <-model.providerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("step cancellation did not release provider after fatal event")
	}
}
