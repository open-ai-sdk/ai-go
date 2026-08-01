package kie

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

// errPollTimeout is returned by Poll when MaxWait elapses with no terminal
// status returned. Callers can use errors.Is to distinguish this from generic
// status errors.
var errPollTimeout = errors.New("kie: poll timeout")

// Poll drives statusFn until either: (a) it reports done=true, (b) it returns
// an error, (c) the deadline elapses, or (d) ctx is cancelled.
//
// Cancellation is checked on every iteration AND inside the inter-tick wait,
// so a cancelled context never wastes more than ~one Interval.
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

	timer := time.NewTimer(0) // first tick immediately
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return zero, errPollTimeout
			}
			return zero, ctx.Err()
		case <-timer.C:
			value, done, err := statusFn(ctx)
			if err != nil {
				return zero, err
			}
			if done {
				return value, nil
			}
			timer.Reset(p.Interval)
		}
	}
}
