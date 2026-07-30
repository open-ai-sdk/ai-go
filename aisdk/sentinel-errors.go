package aisdk

import (
	"errors"
	"fmt"
)

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
	// ErrNoSuchTool is returned when a request names a tool this server has not
	// registered. The concrete *NoSuchToolError that used to wrap it belonged to the
	// deleted tool-loop engine; the sentinel is kept because tool dispatch still needs
	// to classify this case, and a request naming an unknown tool is a 400, not a 500.
	ErrNoSuchTool = errors.New("ai: no such tool")

	// ErrInvalidChunk is returned by a chunk constructor whose arguments cannot form a
	// valid chunk — a dotless custom.kind, an empty data name. These are producer bugs
	// the client cannot detect: its schema accepts both, so the only place they can be
	// caught is here.
	ErrInvalidChunk = errors.New("aisdk: invalid chunk")
)

// invalidChunkf builds an ErrInvalidChunk with context, so errors.Is still matches
// while the message says which constructor rejected what.
func invalidChunkf(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalidChunk}, args...)...)
}
