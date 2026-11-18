package llm_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/transport"
)

func TestDownloadImagePreservesHeadersAndContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "Example/1.0" {
			t.Error(r.Header)
		}
		w.Header().Set("Content-Type", "image/png; profile=example")
		_, _ = io.WriteString(w, "image bytes")
	}))
	defer server.Close()
	data, contentType, err := llm.DownloadImage(
		context.Background(),
		server.URL,
		llm.ImageDownloadOptions{Headers: map[string]string{"User-Agent": "Example/1.0"}},
	)
	if err != nil || string(data) != "image bytes" || contentType != "image/png; profile=example" {
		t.Fatalf("download = %q, %q, %v", data, contentType, err)
	}
}

func TestDownloadImageReportsStatusWithoutResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "private diagnostic data")
	}))
	defer server.Close()
	_, _, err := llm.DownloadImage(context.Background(), server.URL, llm.ImageDownloadOptions{})
	var statusErr *llm.ImageDownloadError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusForbidden ||
		err.Error() != "image download returned HTTP 403" {
		t.Fatalf("error = %v", err)
	}
}

func TestDownloadImageCustomDoerAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := llm.DownloadImage(ctx, "https://example.com/image", llm.ImageDownloadOptions{
		HTTPClient: transport.DoerFunc(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() }),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}
