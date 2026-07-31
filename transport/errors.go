package transport

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
)

const defaultErrorBodyLimit = 64 * 1024

// APIErrorFromResponse converts a non-success HTTP response into a typed
// *aikit.APIError and closes the response body. The raw provider body is
// available only to an explicitly injected debug logger.
func APIErrorFromResponse(
	ctx context.Context,
	provider string,
	resp *http.Response,
) *aikit.APIError {
	defer resp.Body.Close()

	apiErr := aikit.NewAPIError(provider, resp.StatusCode, nil)
	apiErr.RequestID = firstHeader(
		resp.Header,
		"X-Request-Id",
		"Request-Id",
		"X-Amzn-Requestid",
		"Cf-Ray",
		"Anthropic-Request-Id",
	)
	apiErr.RetryAfter = ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now())

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, defaultErrorBodyLimit))
	apiErr.Code, apiErr.Message = parseProviderErrorBody(body)
	LoggerFromContext(ctx).DebugContext(
		ctx,
		"provider error response",
		"provider", provider,
		"status", resp.StatusCode,
		"body", string(body),
		"read_error", readErr,
	)
	return apiErr
}

// ParseRetryAfter parses a Retry-After header containing either delta seconds
// or an HTTP date. now is injected to make HTTP-date handling deterministic.
func ParseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	if retryAt, err := http.ParseTime(value); err == nil {
		if delay := retryAt.Sub(now); delay > 0 {
			return delay
		}
	}
	return 0
}

func aikitTransportError(provider string, err error) *aikit.APIError {
	return aikit.NewAPIError(provider, 0, err)
}

func firstHeader(header http.Header, keys ...string) string {
	for _, key := range keys {
		if value := header.Get(key); value != "" {
			return value
		}
	}
	return ""
}

type providerErrorEnvelope struct {
	Error struct {
		Message string `json:"message"`
		Code    any    `json:"code"`
		Type    string `json:"type"`
		Status  string `json:"status"`
	} `json:"error"`
}

func parseProviderErrorBody(body []byte) (code, message string) {
	var envelope providerErrorEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", ""
	}
	message = envelope.Error.Message
	switch value := envelope.Error.Code.(type) {
	case string:
		code = value
	case float64:
		code = strconv.Itoa(int(value))
	}
	if code == "" {
		code = envelope.Error.Type
	}
	if code == "" {
		code = envelope.Error.Status
	}
	return code, message
}
