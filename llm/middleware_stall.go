package llm

import (
	"context"
	"fmt"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// StallTimeoutError reports that a model produced no decoded stream event for
// the configured duration. It implements Timeout() so generic retry
// classifiers treat it as transient; it is not a context.DeadlineExceeded.
type StallTimeoutError struct {
	// After is the configured stall duration.
	After time.Duration
}

func (e *StallTimeoutError) Error() string {
	return fmt.Sprintf("llm: no stream event for %s", e.After)
}

// Timeout marks the error as a timeout for net.Error-style classification.
func (e *StallTimeoutError) Timeout() bool { return true }

// Temporary is false: a stall is reported as a timeout, not a transient net error.
func (e *StallTimeoutError) Temporary() bool { return false }

// WithStallTimeout wraps m so a stream that decodes no event for d is
// cancelled and terminated with a *StallTimeoutError.
//
// Unlike a byte-level read timeout, the timer resets only when the provider's
// stream yields a decoded event (text, reasoning, tool-call, usage, source,
// file, finish). SSE keepalive comments and blank lines never produce events,
// so they cannot keep a hung generation alive. The timer also covers the wait
// for the first event, including the time Stream takes to return. A
// non-positive d returns m unchanged.
//
// The wrapped stream is cancelled through its context on stall; the caller's
// context is honoured too.
func WithStallTimeout(m Model, d time.Duration) Model {
	if d <= 0 {
		return m
	}
	return wrapModel(m, func(ctx context.Context, req Request) (<-chan aikit.StreamEvent, error) {
		return stallStream(ctx, m, req, d)
	})
}

func stallStream(ctx context.Context, m Model, req Request, d time.Duration) (<-chan aikit.StreamEvent, error) {
	inner, cancel := context.WithCancel(ctx)
	stalled := make(chan struct{})
	timer := time.AfterFunc(d, func() {
		close(stalled)
		cancel()
	})

	events, err := m.Stream(inner, req)
	if err != nil {
		stopped := timer.Stop()
		cancel()
		if !stopped {
			return nil, &StallTimeoutError{After: d}
		}
		return nil, err
	}

	out := make(chan aikit.StreamEvent, cap(events))
	go func() {
		defer close(out)
		defer cancel()
		defer timer.Stop()
		for {
			select {
			case ev, ok := <-events:
				if !ok {
					return
				}
				if !timer.Stop() {
					// The timer already fired: the stall verdict stands.
					<-stalled
					emitErr(ctx, out, &StallTimeoutError{After: d})
					return
				}
				// The timer stays stopped while the event is handed to the
				// consumer, so a slow consumer is never mistaken for a silent
				// provider. It restarts once the provider is being waited on.
				if !forward(ctx, out, ev) {
					return
				}
				timer.Reset(d)
			case <-stalled:
				emitErr(ctx, out, &StallTimeoutError{After: d})
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}
