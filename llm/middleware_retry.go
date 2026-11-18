package llm

import (
	"context"
	"errors"
	"io"
	"math"
	"net"
	"syscall"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// RetryOptions configures [WithRetry]. The zero value performs no retries;
// nothing is defaulted, so every count and duration is the caller's choice.
type RetryOptions struct {
	// MaxRetries is the number of additional attempts after the first. Zero or
	// negative disables retrying.
	MaxRetries int
	// InitialDelay is the wait before the first retry. Zero retries at once.
	InitialDelay time.Duration
	// MaxDelay caps every wait, including a provider Retry-After. Zero means
	// uncapped.
	MaxDelay time.Duration
	// BackoffFactor multiplies the delay on each further retry. Values below 1
	// keep the delay constant.
	BackoffFactor float64
	// Retryable overrides the default classifier. The default retries typed
	// transient failures only: retryable HTTP statuses (see
	// [aikit.RetryableStatusCode]), network timeouts, connection resets, and
	// truncated responses. Provider rejections (other 4xx statuses) are never
	// retried, even when Retryable returns true.
	Retryable func(error) bool
	// OnRetry is called before each wait with the 1-based retry number and the
	// error that triggered it. With EmitRetryEvents, the retry boundary is sent
	// before this hook runs; consumers should use that event for ordered state
	// changes rather than synchronizing with the hook.
	OnRetry func(attempt int, err error)
	// CommitOn reports whether a non-error event commits the current attempt,
	// after which failures are never retried. Nil uses the existing content
	// policy: text, reasoning, tool-call delta, source, or file. Finish always
	// commits, regardless of the callback.
	CommitOn func(aikit.StreamEvent) bool
	// EmitRetryEvents forwards pre-commit events immediately instead of
	// buffering and discarding them on failure. Before each retry it emits a
	// StreamEventRetry whose Error is the triggering cause, then calls OnRetry
	// and waits. Startup failures and exhausted retries are delivered as error
	// events on the returned channel. False preserves buffered Stream behavior.
	EmitRetryEvents bool
}

// WithRetry wraps m so a failed call is retried only before its commit point.
//
// By default a failure is retried when it happens before Stream returns a
// channel, or as an error event before the first content event (text, reasoning,
// tool-call delta, source, or file). Events seen before that point (for example
// usage) are discarded with the failed attempt. CommitOn may replace that
// content policy; a finish event always commits. Once committed, later errors
// are passed through and never retried.
//
// EmitRetryEvents opts into immediate forwarding, including events from failed
// attempts. A retry boundary separates those events from the next attempt; its
// Error is informational, not a terminal error. This mode returns a channel
// immediately, with startup and terminal failures delivered as error events.
// Failed attempts are cancelled and their channels joined before the boundary
// is sent. Provider rejections are never retried. Waiting between attempts and
// every attempt honour the caller's context.
func WithRetry(m Model, opts RetryOptions) Model {
	if opts.MaxRetries <= 0 {
		return m
	}
	return wrapModel(m, func(ctx context.Context, req Request) (<-chan aikit.StreamEvent, error) {
		return retryStream(ctx, m, req, opts)
	})
}

func retryStream(ctx context.Context, m Model, req Request, opts RetryOptions) (<-chan aikit.StreamEvent, error) {
	r := &retrier{opts: opts}
	if opts.EmitRetryEvents {
		return r.lifecycleStream(ctx, m, req), nil
	}
	for attempt := 0; ; attempt++ {
		attemptCtx, cancel := context.WithCancel(ctx)
		events, err := m.Stream(attemptCtx, req)
		if err == nil {
			prefix, failure := r.awaitContent(attemptCtx, events)
			if failure == nil {
				return r.passthrough(ctx, cancel, prefix, events), nil
			}
			cancel()
			err = failure
		} else {
			cancel()
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !r.shouldRetry(err) || attempt >= opts.MaxRetries {
			return nil, err
		}
		if err := r.wait(ctx, attempt+1, err); err != nil {
			return nil, err
		}
	}
}

type retrier struct{ opts RetryOptions }

func (r *retrier) commits(ev aikit.StreamEvent) bool {
	if r.opts.CommitOn != nil {
		return r.opts.CommitOn(ev)
	}
	return isContentEvent(ev)
}

// awaitContent buffers events until the first committing event (or a finish,
// or the end of the stream), then returns them. A non-nil failure means the
// attempt failed before committing.
func (r *retrier) awaitContent(
	ctx context.Context,
	events <-chan aikit.StreamEvent,
) (buffered []aikit.StreamEvent, failure error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case ev, ok := <-events:
			if !ok {
				return buffered, nil
			}
			if ev.Type == aikit.StreamEventError {
				if ev.Error == nil {
					return nil, errors.New("llm: stream error event without an error")
				}
				return nil, ev.Error
			}
			buffered = append(buffered, ev)
			if r.commits(ev) || ev.Type == aikit.StreamEventFinish {
				return buffered, nil
			}
		}
	}
}

// lifecycleStream serializes all attempts and retry boundaries through one
// unbuffered channel, so a boundary reaches the consumer before waiting or
// starting another attempt. Only the terminal error is forwarded as an error
// event; errors that will be retried are carried by the retry boundary instead.
func (r *retrier) lifecycleStream(ctx context.Context, m Model, req Request) <-chan aikit.StreamEvent {
	out := make(chan aikit.StreamEvent)
	go func() {
		defer close(out)
		for attempt := 0; ; attempt++ {
			if ctx.Err() != nil {
				return
			}
			attemptCtx, cancel := context.WithCancel(ctx)
			events, err := m.Stream(attemptCtx, req)
			var failure aikit.StreamEvent
			var committed bool
			if err != nil {
				failure = aikit.StreamEvent{Type: aikit.StreamEventError, Error: err}
			} else {
				failure, committed = r.forwardAttempt(ctx, out, events)
			}
			// Model's cancellation contract requires producers to close their
			// channels. Drain after cancellation to join the old attempt and
			// discard any already-buffered or in-flight trailing events.
			cancel()
			if events != nil {
				for range events {
				}
			}
			if ctx.Err() != nil || failure.Type != aikit.StreamEventError {
				return
			}
			if committed || attempt >= r.opts.MaxRetries || !r.shouldRetry(failure.Error) {
				forward(ctx, out, failure)
				return
			}
			if !forward(ctx, out, aikit.StreamEvent{Type: aikit.StreamEventRetry, Error: failure.Error}) {
				return
			}
			if ctx.Err() != nil {
				return
			}
			if err := r.wait(ctx, attempt+1, failure.Error); err != nil {
				return
			}
		}
	}()
	return out
}

func (r *retrier) forwardAttempt(
	ctx context.Context,
	out chan<- aikit.StreamEvent,
	events <-chan aikit.StreamEvent,
) (failure aikit.StreamEvent, committed bool) {
	for {
		if ctx.Err() != nil {
			return aikit.StreamEvent{}, committed
		}
		select {
		case <-ctx.Done():
			return aikit.StreamEvent{}, committed
		case ev, ok := <-events:
			if !ok {
				return aikit.StreamEvent{}, committed
			}
			if ev.Type == aikit.StreamEventError {
				if ev.Error == nil {
					ev.Error = errors.New("llm: stream error event without an error")
				}
				return ev, committed
			}
			if !forward(ctx, out, ev) {
				return aikit.StreamEvent{}, committed
			}
			if !committed && (ev.Type == aikit.StreamEventFinish || r.commits(ev)) {
				committed = true
			}
		}
	}
}

// passthrough replays the committed prefix, then forwards the rest of events.
func (r *retrier) passthrough(
	ctx context.Context,
	cancel context.CancelFunc,
	prefix []aikit.StreamEvent,
	events <-chan aikit.StreamEvent,
) <-chan aikit.StreamEvent {
	out := make(chan aikit.StreamEvent, cap(events)+len(prefix))
	go func() {
		defer close(out)
		defer cancel()
		for _, ev := range prefix {
			if !forward(ctx, out, ev) {
				return
			}
		}
		for {
			select {
			case ev, ok := <-events:
				if !ok || !forward(ctx, out, ev) {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

func (r *retrier) shouldRetry(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var apiErr *aikit.APIError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 &&
			!aikit.RetryableStatusCode(apiErr.StatusCode) {
			return false
		}
		if apiErr.StatusCode == 0 && apiErr.Code == "invalid_request_error" {
			return false
		}
	}
	if r.opts.Retryable != nil {
		return r.opts.Retryable(err)
	}
	return defaultRetryable(err, apiErr)
}

func defaultRetryable(err error, apiErr *aikit.APIError) bool {
	if apiErr != nil && (apiErr.StatusCode != 0 || apiErr.Code != "") {
		return apiErr.Retryable()
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED)
}

func (r *retrier) wait(ctx context.Context, attempt int, cause error) error {
	if r.opts.OnRetry != nil {
		r.opts.OnRetry(attempt, cause)
	}
	delay := r.delay(attempt, cause)
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (r *retrier) delay(attempt int, cause error) time.Duration {
	var delay time.Duration
	var apiErr *aikit.APIError
	if errors.As(cause, &apiErr) && apiErr.RetryAfter > 0 {
		delay = apiErr.RetryAfter
	} else {
		factor := r.opts.BackoffFactor
		if factor < 1 {
			factor = 1
		}
		delay = time.Duration(float64(r.opts.InitialDelay) * math.Pow(factor, float64(attempt-1)))
	}
	if r.opts.MaxDelay > 0 && delay > r.opts.MaxDelay {
		delay = r.opts.MaxDelay
	}
	return delay
}
