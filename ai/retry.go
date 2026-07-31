package ai

import (
	"context"
	"time"

	"github.com/open-ai-sdk/ai-go/transport"
)

// RetryConfig controls retry behavior for transient LLM provider errors.
type RetryConfig = transport.RetryConfig

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
	policy transport.RetryPolicy
}

func newRetryModel(inner LanguageModel, config RetryConfig) *retryModel {
	return &retryModel{
		inner:  inner,
		policy: transport.NewRetryPolicy(config),
	}
}

func (m *retryModel) ModelID() string { return m.inner.ModelID() }

func (m *retryModel) Stream(
	ctx context.Context,
	req LanguageModelRequest,
) (<-chan StreamEvent, error) {
	var lastErr error
	for attempt := 0; attempt <= m.policy.MaxRetries(); attempt++ {
		if attempt > 0 {
			m.policy.Notify(attempt, lastErr)
			if err := transport.Wait(
				ctx,
				m.policy.Delay(attempt, lastErr),
			); err != nil {
				return nil, err
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

// isRetryable reports whether err is a transient failure worth retrying. It
// classifies by type and status code — never by message text — so a 400 whose
// body happens to echo "EOF" or "timeout" back to the caller does not trigger a
// retry of a billable, non-idempotent request.
func isRetryable(err error) bool {
	return transport.IsRetryable(err)
}

// retryAfterFromError returns the provider-advertised Retry-After delay carried
// on a typed *APIError, or 0 when absent. The old string parser is gone: the
// delay now rides the parsed header, not the error message.
func retryAfterFromError(err error) time.Duration {
	return transport.RetryAfter(err)
}
