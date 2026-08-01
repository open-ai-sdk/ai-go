package anthropic

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

func TestLanguageModel_StreamPreservesAnthropicAPIErrorIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		req *http.Request,
	) {
		if req.URL.Path != "/v1/messages" {
			t.Errorf("request path = %q, want /v1/messages", req.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"error":{"type":"rate_limit_error","message":"slow down"}}`))
	}))
	defer server.Close()

	model := NewLanguageModel("claude-test", Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	_, err := model.Stream(context.Background(), llm.Request{})
	if err == nil {
		t.Fatal("expected provider API error")
	}
	var apiErr *aikit.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T %v, want *aikit.APIError", err, err)
	}
	if apiErr.Provider != "anthropic" {
		t.Errorf("provider = %q, want anthropic", apiErr.Provider)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", apiErr.StatusCode, http.StatusTooManyRequests)
	}
	if apiErr.Code != "rate_limit_error" {
		t.Errorf("code = %q, want rate_limit_error", apiErr.Code)
	}
}
