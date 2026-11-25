package fal

import (
	"context"
	"errors"
	"time"
)

// pollerStatusFn is invoked once per tick; returning done=true ends the loop.
type pollerStatusFn[T any] func(ctx context.Context) (value T, done bool, err error)

// poller drives a context-aware polling loop with both a fixed tick interval
// and a wall-clock max-wait.
type poller struct {
	Interval time.Duration
	MaxWait  time.Duration
}

// errPollTimeout is returned by poll when MaxWait elapses with no terminal
// status returned.
var errPollTimeout = errors.New("fal: poll timeout")

// poll drives statusFn until it reports done=true, returns an error, the
// deadline elapses, or ctx is cancelled. The first tick runs immediately.
func poll[T any](parent context.Context, p poller, statusFn pollerStatusFn[T]) (T, error) {
	var zero T
	if p.Interval <= 0 {
		p.Interval = defaultPollInterval
	}
	if p.MaxWait <= 0 {
		p.MaxWait = defaultPollTimeout
	}

	ctx, cancel := context.WithTimeout(parent, p.MaxWait)
	defer cancel()

	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) && parent.Err() == nil {
				return zero, errPollTimeout
			}
			return zero, ctx.Err()
		case <-timer.C:
			value, done, err := statusFn(ctx)
			if err != nil {
				// A status request cut off by the poll deadline is a poll
				// timeout, not a transport failure.
				if parent.Err() == nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
					return zero, errPollTimeout
				}
				return zero, err
			}
			if done {
				return value, nil
			}
			timer.Reset(p.Interval)
		}
	}
}
