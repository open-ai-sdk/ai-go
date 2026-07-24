package ai

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
)

// RetryConfig controls retry behavior for transient LLM provider errors.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts. Default: 2.
	// Use a pointer to distinguish "unset" from explicit 0.
	MaxRetries *int
	// InitialDelay is the base delay before the first retry. Default: 1s.
	InitialDelay time.Duration
	// MaxDelay caps the exponential backoff. Default: 60s.
	MaxDelay time.Duration
	// BackoffFactor multiplies the delay each retry. Default: 2.0.
	BackoffFactor float64
	// Jitter adds randomness to prevent thundering herd. Default: true.
	Jitter *bool
	// OnRetry is called before each retry attempt with the attempt number (1-based)
	// and the error that triggered it. Optional.
	OnRetry func(attempt int, err error)
}

// retryDefaults holds the resolved retry configuration with all defaults applied.
type retryDefaults struct {
	maxRetries    int
	initialDelay  time.Duration
	maxDelay      time.Duration
	backoffFactor float64
	jitter        bool
	onRetry       func(attempt int, err error)
}

func (c RetryConfig) resolve() retryDefaults {
	d := retryDefaults{
		maxRetries:    2,
		initialDelay:  time.Second,
		maxDelay:      60 * time.Second,
		backoffFactor: 2.0,
		jitter:        true,
		onRetry:       c.OnRetry,
	}
	if c.MaxRetries != nil {
		d.maxRetries = *c.MaxRetries
	}
	if c.InitialDelay > 0 {
		d.initialDelay = c.InitialDelay
	}
	if c.MaxDelay > 0 {
		d.maxDelay = c.MaxDelay
	}
	if c.BackoffFactor > 0 {
		d.backoffFactor = c.BackoffFactor
	}
	if c.Jitter != nil {
		d.jitter = *c.Jitter
	}
	return d
}

// intPtr returns a pointer to n.
func intPtr(n int) *int { return &n }

// boolPtr returns a pointer to b.
func boolPtr(b bool) *bool { return &b }

// WithMaxRetries returns an Option that enables retry with the given max attempts.
// Uses default backoff settings (1s initial, 2x factor, 60s max, jitter enabled).
// Pass 0 to explicitly disable retries.
func WithMaxRetries(n int) Option {
	return WithRetry(RetryConfig{MaxRetries: intPtr(n), Jitter: boolPtr(true)})
}

// WithRetry returns an Option that stores retry config for deferred model wrapping.
func WithRetry(config RetryConfig) Option {
	return func(r *GenerateTextRequest) {
		mw := func(model LanguageModel) LanguageModel {
			return newRetryModel(model, config)
		}
		r.Middlewares = append(r.Middlewares, mw)
	}
}

// retryModel wraps a LanguageModel with retry logic.
type retryModel struct {
	inner  LanguageModel
	config retryDefaults
}

func newRetryModel(inner LanguageModel, config RetryConfig) *retryModel {
	return &retryModel{inner: inner, config: config.resolve()}
}

func (m *retryModel) ModelID() string { return m.inner.ModelID() }

func (m *retryModel) Stream(
	ctx context.Context,
	req LanguageModelRequest,
) (<-chan StreamEvent, error) {
	var lastErr error
	for attempt := 0; attempt <= m.config.maxRetries; attempt++ {
		if attempt > 0 {
			if m.config.onRetry != nil {
				m.config.onRetry(attempt, lastErr)
			}
			delay := m.calculateDelay(attempt)
			if retryAfter := retryAfterFromError(lastErr); retryAfter > 0 {
				delay = retryAfter
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		ch, err := m.inner.Stream(ctx, req)
		if err == nil {
			return ch, nil
		}
		if !isRetryable(err) {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

func (m *retryModel) calculateDelay(attempt int) time.Duration {
	delay := float64(m.config.initialDelay) * math.Pow(
		m.config.backoffFactor, float64(attempt-1),
	)
	if delay > float64(m.config.maxDelay) {
		delay = float64(m.config.maxDelay)
	}
	if m.config.jitter {
		delay = delay * (0.5 + cryptoFloat64()*0.5)
	}
	return time.Duration(delay)
}

// isRetryable reports whether err is a transient failure worth retrying. It
// classifies by type and status code — never by message text — so a 400 whose
// body happens to echo "EOF" or "timeout" back to the caller does not trigger a
// retry of a billable, non-idempotent request.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	// Cancellation and deadlines are the caller's decision, never retried.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Typed provider failures classify by their status code.
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Retryable()
	}
	// Transient network failures: request timeouts, truncated responses, and
	// refused/reset connections are all worth another attempt.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	return false
}

// retryAfterFromError returns the provider-advertised Retry-After delay carried
// on a typed *APIError, or 0 when absent. The old string parser is gone: the
// delay now rides the parsed header, not the error message.
func retryAfterFromError(err error) time.Duration {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.RetryAfter
	}
	return 0
}

// cryptoFloat64 returns a cryptographically random float64 in [0, 1).
func cryptoFloat64() float64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return float64(binary.LittleEndian.Uint64(b[:])>>11) / (1 << 53)
}
