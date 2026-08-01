package transport

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net"
	"syscall"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// RetryConfig controls retry behavior for transient transport failures.
type RetryConfig struct {
	MaxRetries    *int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
	Jitter        *bool
	OnRetry       func(attempt int, err error)
}

// RetryPolicy is a resolved retry configuration.
type RetryPolicy struct {
	maxRetries    int
	initialDelay  time.Duration
	maxDelay      time.Duration
	backoffFactor float64
	jitter        bool
	onRetry       func(attempt int, err error)
}

// NewRetryPolicy resolves config defaults.
func NewRetryPolicy(config RetryConfig) RetryPolicy {
	policy := RetryPolicy{
		maxRetries:    2,
		initialDelay:  time.Second,
		maxDelay:      60 * time.Second,
		backoffFactor: 2,
		jitter:        true,
		onRetry:       config.OnRetry,
	}
	if config.MaxRetries != nil {
		policy.maxRetries = *config.MaxRetries
	}
	if config.InitialDelay > 0 {
		policy.initialDelay = config.InitialDelay
	}
	if config.MaxDelay > 0 {
		policy.maxDelay = config.MaxDelay
	}
	if config.BackoffFactor > 0 {
		policy.backoffFactor = config.BackoffFactor
	}
	if config.Jitter != nil {
		policy.jitter = *config.Jitter
	}
	return policy
}

// MaxRetries returns the configured number of retries after the first attempt.
func (p RetryPolicy) MaxRetries() int {
	return p.maxRetries
}

// Notify invokes the optional retry callback.
func (p RetryPolicy) Notify(attempt int, err error) {
	if p.onRetry != nil {
		p.onRetry(attempt, err)
	}
}

// Delay returns the delay before attempt, where the first retry is attempt 1.
// A typed Retry-After value takes precedence over exponential backoff.
func (p RetryPolicy) Delay(attempt int, err error) time.Duration {
	if retryAfter := RetryAfter(err); retryAfter > 0 {
		return retryAfter
	}
	delay := float64(p.initialDelay) *
		math.Pow(p.backoffFactor, float64(attempt-1))
	if delay > float64(p.maxDelay) {
		delay = float64(p.maxDelay)
	}
	if p.jitter {
		delay *= 0.5 + cryptoFloat64()*0.5
	}
	return time.Duration(delay)
}

// Wait blocks for delay or returns the context error.
func Wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// IsRetryable reports whether err is transient. It uses typed errors and
// errors.Is/errors.As only; provider error strings are never inspected.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var apiErr *aikit.APIError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode != 0 {
			return apiErr.Retryable()
		}
		// A status-zero APIError wraps a failure that happened before an HTTP
		// response existed. Continue through the wrapped cause so timeouts,
		// resets, and truncated responses retain their retry classification.
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return true
	}
	return errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET)
}

// RetryAfter extracts typed provider retry metadata.
func RetryAfter(err error) time.Duration {
	var apiErr *aikit.APIError
	if errors.As(err, &apiErr) {
		return apiErr.RetryAfter
	}
	return 0
}

func cryptoFloat64() float64 {
	var random [8]byte
	_, _ = rand.Read(random[:])
	return float64(binary.LittleEndian.Uint64(random[:])>>11) / (1 << 53)
}
