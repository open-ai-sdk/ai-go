package aikit

import (
	"fmt"
	"strings"
	"time"
)

// APIError is a provider HTTP failure surfaced as a value, not a string.
// Parsed code/message and request ID are retained by default. RawBody is
// populated only when diagnostic capture is explicitly enabled; Error never
// includes it.
type APIError struct {
	Provider   string
	StatusCode int
	RequestID  string
	Code       string // provider error code, when parsed
	Message    string // provider message, when parsed — never the raw body
	// RawBody is opt-in diagnostic data. It may contain sensitive provider
	// fields and must not be logged without application-level redaction.
	RawBody []byte
	// RetryAfter is taken from the Retry-After response header; 0 when absent.
	RetryAfter time.Duration
	err        error // wrapped transport error, if any
}

// NewAPIError builds an *APIError. Pass wrapped=nil when there is no underlying
// transport error (a non-2xx HTTP status is itself the failure).
func NewAPIError(provider string, statusCode int, wrapped error) *APIError {
	return &APIError{Provider: provider, StatusCode: statusCode, err: wrapped}
}

func (e *APIError) Error() string {
	var b strings.Builder
	b.WriteString(e.Provider)
	fmt.Fprintf(&b, ": status %d", e.StatusCode)
	if e.Code != "" {
		fmt.Fprintf(&b, " (%s)", e.Code)
	}
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, " [request %s]", e.RequestID)
	}
	if e.err != nil {
		b.WriteString(": ")
		b.WriteString(e.err.Error())
	}
	return b.String()
}

// Unwrap exposes the wrapped transport error for errors.Is/As traversal.
func (e *APIError) Unwrap() error { return e.err }

// Is maps HTTP status (and parsed context-length codes) to the package
// sentinels so consumers can write errors.Is(err, ErrRateLimited) without
// inspecting the concrete type.
func (e *APIError) Is(target error) bool {
	switch target {
	case ErrRateLimited:
		return e.StatusCode == 429
	case ErrUnauthorized:
		return e.StatusCode == 401 || e.StatusCode == 403
	case ErrContextLength:
		// Detected from the parsed error code/message, not the raw body. 400 is
		// too generic to map on status alone, so a code/message signal is required.
		return isContextLengthSignal(e.Code) || isContextLengthSignal(e.Message)
	}
	return false
}

// Retryable reports whether a failure is transient. Stream errors have no HTTP
// status; only an explicit server_error code makes such an error retryable.
func (e *APIError) Retryable() bool {
	return RetryableStatusCode(e.StatusCode) || (e.StatusCode == 0 && e.Code == "server_error")
}

// RetryableStatusCode reports whether an HTTP status code is transient and worth
// retrying: 429, 500, 502, 503, and Anthropic's 529 overload.
func RetryableStatusCode(code int) bool {
	switch code {
	case 429, 500, 502, 503, 529:
		return true
	}
	return false
}

func isContextLengthSignal(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "context_length_exceeded") ||
		strings.Contains(s, "context length") ||
		strings.Contains(s, "maximum context")
}
