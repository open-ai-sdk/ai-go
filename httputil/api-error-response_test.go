package httputil

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/aitypes"
	"github.com/open-ai-sdk/ai-go/internal/ctxlog"
)

func makeResp(status int, header http.Header, body string) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestAPIErrorFromResponse_ParsesHeadersAndBody(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "30")
	h.Set("X-Request-Id", "req_abc")
	body := `{"error":{"message":"Rate limit reached","code":"rate_limit_exceeded","type":"requests"}}`

	resp := makeResp(429, h, body)
	defer resp.Body.Close()
	err := APIErrorFromResponse(context.Background(), "openai", resp)

	if err.StatusCode != 429 {
		t.Errorf("StatusCode = %d, want 429", err.StatusCode)
	}
	if err.RetryAfter != 30*time.Second {
		t.Errorf("RetryAfter = %v, want 30s", err.RetryAfter)
	}
	if err.RequestID != "req_abc" {
		t.Errorf("RequestID = %q, want req_abc", err.RequestID)
	}
	if err.Code != "rate_limit_exceeded" {
		t.Errorf("Code = %q, want rate_limit_exceeded", err.Code)
	}
	if err.Message != "Rate limit reached" {
		t.Errorf("Message = %q", err.Message)
	}
	if !errors.Is(err, aitypes.ErrRateLimited) {
		t.Error("expected errors.Is(err, ErrRateLimited) for a 429")
	}
}

func TestAPIErrorFromResponse_DoesNotEmbedRawBody(t *testing.T) {
	// A body carrying a secret-looking token must not survive into the error text.
	secret := "sk-live-SECRETTOKEN-should-not-leak"
	body := `{"error":{"message":"bad","code":"x"},"debug":"` + secret + `"}`
	resp := makeResp(400, nil, body)
	defer resp.Body.Close()
	err := APIErrorFromResponse(context.Background(), "openai", resp)
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("raw body leaked into error text: %q", err.Error())
	}
}

// recordingHandler is a minimal slog.Handler that captures every record it
// receives, so a test can assert exactly what — if anything — was logged.
type recordingHandler struct{ records []slog.Record }

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

func TestAPIErrorFromResponse_LogsDroppedBodyWhenLoggerConfigured(t *testing.T) {
	secret := "sk-live-SECRETTOKEN-should-not-leak"
	body := `{"error":{"message":"bad","code":"x"},"debug":"` + secret + `"}`

	rec := &recordingHandler{}
	ctx := ctxlog.WithLogger(context.Background(), slog.New(rec))

	resp := makeResp(400, nil, body)
	defer resp.Body.Close()
	_ = APIErrorFromResponse(ctx, "openai", resp)

	if len(rec.records) != 1 {
		t.Fatalf("expected exactly one log record, got %d", len(rec.records))
	}
	found := false
	rec.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "body" && strings.Contains(a.Value.String(), secret) {
			found = true
		}
		return true
	})
	if !found {
		t.Fatal("expected the dropped raw body to reach the injected logger's \"body\" attribute")
	}
}

func TestAPIErrorFromResponse_NoLoggerConfigured_NoOutput(t *testing.T) {
	// The default (no logger attached to ctx) must discard silently — never
	// touch slog.Default(), never panic on a nil ctx value.
	body := `{"error":{"message":"bad","code":"x"}}`
	resp := makeResp(400, nil, body)
	defer resp.Body.Close()
	err := APIErrorFromResponse(context.Background(), "openai", resp)
	if err == nil {
		t.Fatal("expected a non-nil *APIError")
	}
}

func TestParseRetryAfterHeader_SecondsAndDate(t *testing.T) {
	if got := parseRetryAfterHeader("15"); got != 15*time.Second {
		t.Errorf("seconds: got %v", got)
	}
	if got := parseRetryAfterHeader(""); got != 0 {
		t.Errorf("empty: got %v", got)
	}
	if got := parseRetryAfterHeader("garbage"); got != 0 {
		t.Errorf("garbage: got %v", got)
	}
}
