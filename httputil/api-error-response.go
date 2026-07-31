package httputil

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/internal/ctxlog"
)

// defaultErrorBodyLimit bounds how much of an error response body is read for
// parsing. The body is attacker-influenced and discarded after parsing; only the
// extracted code/message survive into the typed error.
const defaultErrorBodyLimit = 64 * 1024

// APIErrorFromResponse converts a non-2xx response into a typed *aikit.APIError.
// It parses the provider error code/message from the (bounded) JSON body, the
// request-ID and Retry-After response headers, then discards the raw body — it is
// never embedded in the error value. resp.Body is read and closed.
//
// The raw body still reaches the caller if they opted into diagnostics: it is
// logged at debug level via the *slog.Logger attached to ctx (see ctxlog),
// which is a no-op unless the caller configured a logger (ai.WithLogger).
// This is the same "off by default, explicit opt-in" policy applied to span
// content — a security-relevant default, not an incidental one.
func APIErrorFromResponse(ctx context.Context, provider string, resp *http.Response) *aikit.APIError {
	defer resp.Body.Close()
	e := aikit.NewAPIError(provider, resp.StatusCode, nil)
	e.RequestID = firstHeader(resp.Header,
		"X-Request-Id", "Request-Id", "X-Amzn-Requestid", "Cf-Ray", "Anthropic-Request-Id")
	if ra := parseRetryAfterHeader(resp.Header.Get("Retry-After")); ra > 0 {
		e.RetryAfter = ra
	}
	// A truncated read still yields a usable (possibly partial) body for
	// best-effort code/message parsing; readErr is surfaced to the debug log
	// rather than dropped, so a partial parse is diagnosable.
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, defaultErrorBodyLimit))
	e.Code, e.Message = parseProviderErrorBody(body)
	ctxlog.FromContext(ctx).DebugContext(ctx, "provider error response",
		"provider", provider, "status", resp.StatusCode, "body", string(body), "read_error", readErr)
	return e
}

// firstHeader returns the first non-empty value among the given header keys.
func firstHeader(h http.Header, keys ...string) string {
	for _, k := range keys {
		if v := h.Get(k); v != "" {
			return v
		}
	}
	return ""
}

// parseRetryAfterHeader parses a Retry-After header value, which is either a
// count of seconds or an HTTP date. Returns 0 when absent or unparseable.
func parseRetryAfterHeader(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// providerErrorEnvelope covers the common { "error": {...} } shape used by
// OpenAI, Anthropic, and Gemini. Fields absent for a given provider stay zero.
type providerErrorEnvelope struct {
	Error struct {
		Message string `json:"message"`
		Code    any    `json:"code"` // string or number depending on provider
		Type    string `json:"type"`
		Status  string `json:"status"`
	} `json:"error"`
}

// parseProviderErrorBody extracts a best-effort code and message from a provider
// error body. An unparseable body yields empty strings — never the raw text.
func parseProviderErrorBody(body []byte) (code, message string) {
	var env providerErrorEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return "", ""
	}
	message = env.Error.Message
	switch c := env.Error.Code.(type) {
	case string:
		code = c
	case float64:
		code = strconv.Itoa(int(c))
	}
	if code == "" {
		code = env.Error.Type
	}
	if code == "" {
		code = env.Error.Status
	}
	return code, message
}
