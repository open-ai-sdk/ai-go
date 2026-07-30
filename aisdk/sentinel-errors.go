package aisdk

import "errors"

// Sentinel errors let consumers classify failures with errors.Is without
// depending on message text or provider internals. *APIError.Is maps HTTP
// status codes to these, so errors.Is(err, ErrRateLimited) is true for any
// provider 429 while the concrete *APIError still carries the detail.
var (
	// ErrRateLimited is matched by any provider 429 (Too Many Requests).
	ErrRateLimited = errors.New("ai: rate limited")
	// ErrContextLength is matched when a provider reports the prompt exceeded the
	// model's context window (by parsed error code, not raw body text).
	ErrContextLength = errors.New("ai: context length exceeded")
	// ErrUnauthorized is matched by a provider 401 or 403.
	ErrUnauthorized = errors.New("ai: unauthorized")
	// ErrNoSuchTool is matched by *NoSuchToolError.
	ErrNoSuchTool = errors.New("ai: no such tool")
)
