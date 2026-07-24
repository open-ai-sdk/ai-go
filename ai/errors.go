package ai

import "github.com/open-ai-sdk/ai-go/aitypes"

// APIError is a provider HTTP failure surfaced as a typed value; the raw
// response body is never embedded. See aitypes.APIError.
type APIError = aitypes.APIError

// Sentinel errors for errors.Is classification. *APIError.Is maps status codes
// to these, so consumers write errors.Is(err, ai.ErrRateLimited).
var (
	ErrRateLimited   = aitypes.ErrRateLimited
	ErrContextLength = aitypes.ErrContextLength
	ErrUnauthorized  = aitypes.ErrUnauthorized
	ErrNoSuchTool    = aitypes.ErrNoSuchTool
)

// NewAPIError builds a typed provider error. Providers usually call
// httputil.APIErrorFromResponse instead, which also parses headers and body.
func NewAPIError(provider string, statusCode int, wrapped error) *APIError {
	return aitypes.NewAPIError(provider, statusCode, wrapped)
}

// RetryableStatusCode reports whether an HTTP status code is transient and worth
// retrying. Exported for provider implementations.
func RetryableStatusCode(code int) bool { return aitypes.RetryableStatusCode(code) }
