package aisdk

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAPIError_IsMapsStatusToSentinels(t *testing.T) {
	cases := []struct {
		status int
		target error
		wantIs bool
	}{
		{429, ErrRateLimited, true},
		{500, ErrRateLimited, false},
		{401, ErrUnauthorized, true},
		{403, ErrUnauthorized, true},
		{429, ErrUnauthorized, false},
	}
	for _, c := range cases {
		err := NewAPIError("p", c.status, nil)
		if got := errors.Is(err, c.target); got != c.wantIs {
			t.Errorf("status %d errors.Is(%v) = %v, want %v", c.status, c.target, got, c.wantIs)
		}
	}
}

func TestAPIError_ContextLengthFromCode(t *testing.T) {
	err := NewAPIError("openai", 400, nil)
	err.Code = "context_length_exceeded"
	if !errors.Is(err, ErrContextLength) {
		t.Error("expected context_length_exceeded code to match ErrContextLength")
	}
	plain := NewAPIError("openai", 400, nil)
	if errors.Is(plain, ErrContextLength) {
		t.Error("a bare 400 must not match ErrContextLength")
	}
}

func TestAPIError_Retryable(t *testing.T) {
	for _, c := range []struct {
		status int
		want   bool
	}{{429, true}, {500, true}, {502, true}, {503, true}, {529, true}, {400, false}, {401, false}} {
		if got := NewAPIError("p", c.status, nil).Retryable(); got != c.want {
			t.Errorf("status %d Retryable() = %v, want %v", c.status, got, c.want)
		}
	}
}

func TestAPIError_ErrorOmitsRawBody(t *testing.T) {
	err := NewAPIError("openai", 401, nil)
	err.RequestID = "req_123"
	err.Code = "invalid_api_key"
	err.Message = "Incorrect API key provided"
	err.RetryAfter = 30 * time.Second
	s := err.Error()
	// The parsed fields are allowed; what must never appear is a raw body, which
	// this type has no field for. Sanity-check the format carries status + code.
	if !strings.Contains(s, "401") || !strings.Contains(s, "invalid_api_key") {
		t.Errorf("Error() = %q, expected status and code", s)
	}
}

func TestAPIError_Unwrap(t *testing.T) {
	inner := errors.New("dial tcp: refused")
	err := NewAPIError("p", 0, inner)
	if !errors.Is(err, inner) {
		t.Error("expected Unwrap to expose the wrapped transport error")
	}
}
