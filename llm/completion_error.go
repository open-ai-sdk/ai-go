package llm

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/transport"
)

// CompletionErrorKind is a stable, provider-neutral completion failure class.
type CompletionErrorKind string

const (
	CompletionErrorKindTransport  CompletionErrorKind = "transport"
	CompletionErrorKindJSONDecode CompletionErrorKind = "json_decode"
	CompletionErrorKindRequest    CompletionErrorKind = "invalid_request"
	CompletionErrorKindResponse   CompletionErrorKind = "invalid_response"
	CompletionErrorKindProvider   CompletionErrorKind = "provider"

	// Longer aliases make the meaning clear at call sites while preserving the
	// concise Request and Response names used by provider implementations.
	CompletionErrorKindInvalidRequest  = CompletionErrorKindRequest
	CompletionErrorKindInvalidResponse = CompletionErrorKindResponse

	// Concise aliases keep provider adapters readable.
	CompletionErrorTransport = CompletionErrorKindTransport
	CompletionErrorJSON      = CompletionErrorKindJSONDecode
	CompletionErrorRequest   = CompletionErrorKindRequest
	CompletionErrorResponse  = CompletionErrorKindResponse
	CompletionErrorProvider  = CompletionErrorKindProvider
)

// CompletionError adds stable classification and operation context to a
// provider-neutral completion failure. Cause remains available through
// errors.Is/errors.As; in particular, an *aikit.APIError is never flattened.
type CompletionError struct {
	Kind      CompletionErrorKind
	Operation string
	Provider  string
	Cause     error
}

func (e *CompletionError) Error() string {
	if e == nil {
		return "llm: completion failed"
	}
	message := "llm: completion"
	if e.Operation != "" {
		message += " " + e.Operation
	}
	if e.Provider != "" {
		message += " (" + e.Provider + ")"
	}
	if e.Kind != "" {
		message += ": " + string(e.Kind)
	}
	if e.Cause != nil {
		message += ": " + e.Cause.Error()
	}
	return message
}

func (e *CompletionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Is permits kind matching with errors.Is(err, &CompletionError{Kind: kind}).
// A zero Kind is not a wildcard; use errors.As to match every completion error.
func (e *CompletionError) Is(target error) bool {
	want, ok := target.(*CompletionError)
	return ok && e != nil && want != nil && want.Kind != "" && e.Kind == want.Kind
}

// Retryable follows the unwrap chain. Cancellation and malformed/invalid data
// are fatal; HTTP retry policy remains owned by APIError.
func (e *CompletionError) Retryable() bool {
	if e == nil || errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
		return false
	}
	var apiErr *aikit.APIError
	if errors.As(e, &apiErr) {
		return apiErr.Retryable()
	}
	return e.Kind == CompletionErrorKindTransport && transport.IsRetryable(e.Cause)
}

// NewCompletionError constructs a classified wrapper without double-wrapping
// an existing CompletionError.
func NewCompletionError(kind CompletionErrorKind, operation, provider string, cause error) error {
	if cause == nil {
		return &CompletionError{Kind: kind, Operation: operation, Provider: provider}
	}
	var existing *CompletionError
	if errors.As(cause, &existing) {
		return cause
	}
	if provider == "" {
		var apiErr *aikit.APIError
		if errors.As(cause, &apiErr) {
			provider = apiErr.Provider
		}
	}
	return &CompletionError{Kind: kind, Operation: operation, Provider: provider, Cause: cause}
}

// WrapCompletionError is the provider-adapter spelling of NewCompletionError.
func WrapCompletionError(kind CompletionErrorKind, operation, provider string, cause error) error {
	return NewCompletionError(kind, operation, provider, cause)
}

func completionErrorKind(cause error, fallback CompletionErrorKind) CompletionErrorKind {
	if cause == nil {
		return fallback
	}
	var apiErr *aikit.APIError
	if errors.As(cause, &apiErr) {
		return CompletionErrorKindProvider
	}
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(cause, &syntaxErr) || errors.As(cause, &typeErr) {
		return CompletionErrorKindJSONDecode
	}
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) ||
		transport.IsRetryable(cause) {
		return CompletionErrorKindTransport
	}
	return fallback
}

func wrapCompletionError(cause error, fallback CompletionErrorKind, operation string) error {
	if cause == nil {
		return nil
	}
	return NewCompletionError(completionErrorKind(cause, fallback), operation, "", cause)
}

func invalidCompletionResponse(operation, reason string) error {
	return &CompletionError{
		Kind:      CompletionErrorKindResponse,
		Operation: operation,
		Cause:     errors.New(reason),
	}
}
