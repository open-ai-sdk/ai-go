package gemini

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

func TestGeminiNonStreamingErrors_RedactRawBody(t *testing.T) {
	const secret = "raw-response-secret"
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		_ *http.Request,
	) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(
			`{"error":{"message":"invalid request","code":"bad_request"},"debug":"` +
				secret + `"}`,
		))
	}))
	defer server.Close()

	t.Run("embedding", func(t *testing.T) {
		model := newTestEmbeddingModel(t, server)
		_, err := model.Embed(context.Background(), "hello")
		assertRedactedAPIError(t, err, secret)
	})

	t.Run("image", func(t *testing.T) {
		model := NewImageModel("gemini-image", Config{
			APIKey:  "test-key",
			BaseURL: server.URL,
		})
		_, err := model.Generate(
			context.Background(),
			llm.GenerateImageRequest{Prompt: "hello"},
		)
		assertRedactedAPIError(t, err, secret)
	})
}

func assertRedactedAPIError(t *testing.T, err error, secret string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected provider error")
	}
	var apiErr *aikit.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *aikit.APIError", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("raw response body leaked into error: %q", err)
	}
}
