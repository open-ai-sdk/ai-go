package httputil

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/aitypes"
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

	err := APIErrorFromResponse("openai", makeResp(429, h, body))

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
	err := APIErrorFromResponse("openai", makeResp(400, nil, body))
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("raw body leaked into error text: %q", err.Error())
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
