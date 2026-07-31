package transport

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func response(status int, header http.Header, body string) *http.Response {
	if header == nil {
		header = make(http.Header)
	}
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestAPIErrorFromResponse_UsesRetryAfterHeader(t *testing.T) {
	header := make(http.Header)
	header.Set("Retry-After", "30")
	header.Set("X-Request-Id", "req_abc")
	resp := response( //nolint:bodyclose // APIErrorFromResponse owns and closes it.
		http.StatusTooManyRequests,
		header,
		`{"error":{"message":"Rate limit reached","code":"rate_limit_exceeded"}}`,
	)

	err := APIErrorFromResponse(context.Background(), "openai", resp)

	if err.RetryAfter != 30*time.Second {
		t.Errorf("RetryAfter = %v, want 30s", err.RetryAfter)
	}
	if err.RequestID != "req_abc" {
		t.Errorf("RequestID = %q, want req_abc", err.RequestID)
	}
	if err.Code != "rate_limit_exceeded" || err.Message != "Rate limit reached" {
		t.Errorf("parsed error = code %q message %q", err.Code, err.Message)
	}
	if !errors.Is(err, aikit.ErrRateLimited) {
		t.Error("expected rate-limit sentinel")
	}
}

func TestParseRetryAfter_HTTPDateUsesInjectedClock(t *testing.T) {
	now := time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC)
	retryAt := now.Add(45 * time.Second).Format(http.TimeFormat)
	if got := ParseRetryAfter(retryAt, now); got != 45*time.Second {
		t.Fatalf("ParseRetryAfter() = %v, want 45s", got)
	}
}

type recordingHandler struct {
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *recordingHandler) Handle(_ context.Context, record slog.Record) error {
	h.records = append(h.records, record.Clone())
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h *recordingHandler) WithGroup(string) slog.Handler {
	return h
}

func TestAPIErrorFromResponse_RedactsErrorAndLogsRawBody(t *testing.T) {
	const secret = "sk-live-secret-that-must-not-leak"
	handler := &recordingHandler{}
	ctx := WithLogger(
		context.Background(),
		slog.New(handler),
	)
	resp := response( //nolint:bodyclose // APIErrorFromResponse owns and closes it.
		http.StatusBadRequest,
		nil,
		`{"error":{"message":"bad","code":"invalid"},"debug":"`+secret+`"}`,
	)

	err := APIErrorFromResponse(ctx, "openai", resp)

	if strings.Contains(err.Error(), secret) {
		t.Fatalf("raw body leaked into error: %q", err)
	}
	if len(handler.records) != 1 {
		t.Fatalf("log record count = %d, want 1", len(handler.records))
	}
	found := false
	handler.records[0].Attrs(func(attr slog.Attr) bool {
		if attr.Key == "body" && strings.Contains(attr.Value.String(), secret) {
			found = true
		}
		return true
	})
	if !found {
		t.Fatal("raw body did not reach injected debug logger")
	}
}
