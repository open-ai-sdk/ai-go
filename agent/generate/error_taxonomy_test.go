package generate

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func TestIsRetryable_ClassifiesByTypeNotText(t *testing.T) {
	if !isRetryable(NewAPIError("p", 429, nil)) {
		t.Error("429 APIError should be retryable")
	}
	// A 400 whose parsed message echoes "EOF" must NOT retry: classification is
	// by status code, never by substring. This is the attacker-echo guard.
	bad := NewAPIError("p", 400, nil)
	bad.Message = "invalid request: unexpected EOF while reading prompt"
	if isRetryable(bad) {
		t.Error("a 400 must not be retryable even when its message contains EOF")
	}
	if !isRetryable(io.ErrUnexpectedEOF) {
		t.Error("io.ErrUnexpectedEOF (truncated response) should be retryable")
	}
	if isRetryable(context.Canceled) || isRetryable(context.DeadlineExceeded) {
		t.Error("cancellation/deadline must never be retryable")
	}
	if isRetryable(nil) {
		t.Error("nil is not retryable")
	}
}

func TestRetryAfterFromError(t *testing.T) {
	e := NewAPIError("p", 429, nil)
	e.RetryAfter = 30 * time.Second
	if got := retryAfterFromError(e); got != 30*time.Second {
		t.Errorf("got %v, want 30s", got)
	}
	if got := retryAfterFromError(errors.New("plain")); got != 0 {
		t.Errorf("non-APIError should yield 0, got %v", got)
	}
}

// apiErrModel is a fake LanguageModel that fails its Stream with a typed error.
type apiErrModel struct{ err error }

func (apiErrModel) ModelID() string { return "apierr" }
func (m apiErrModel) Stream(context.Context, LanguageModelRequest) (<-chan StreamEvent, error) {
	return nil, m.err
}

func TestGenerateText_APIErrorSurvivesToolLoop(t *testing.T) {
	apiErr := NewAPIError("openai", 429, nil)
	_, err := GenerateText(context.Background(), GenerateTextRequest{
		Model:    apiErrModel{err: apiErr},
		Messages: []Message{UserMessage("hi")},
		MaxSteps: 1,
	})
	if err == nil {
		t.Fatal("expected an error from the failing model")
	}
	var got *APIError
	if !errors.As(err, &got) {
		t.Fatalf("errors.As(err, *APIError) failed: %T %v", err, err)
	}
	if got.StatusCode != 429 {
		t.Errorf("recovered APIError status = %d, want 429", got.StatusCode)
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Error("errors.Is(err, ErrRateLimited) must hold for a provider 429 raised in the loop")
	}
}
