package kie

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

func TestLanguageApplicationFailureKeepsProviderClassWithoutHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"code":524,"msg":"upstream timed out"}`)
	}))
	defer server.Close()
	model := NewProvider("example-key", WithBaseURL(server.URL)).Responses("example-model")
	response, err := model.Complete(context.Background(), llm.Request{})
	var api *aikit.APIError
	var completion *llm.CompletionError
	if !errors.As(err, &api) || api.StatusCode != 0 || api.Code != "524" || api.Message != "upstream timed out" {
		t.Fatalf("provider error: %v", err)
	}
	if !errors.As(err, &completion) || completion.Kind != llm.CompletionErrorKindProvider {
		t.Fatalf("completion classification: %v", err)
	}
	if response == nil || response.RawResponse.(map[string]any)["code"] != float64(524) {
		t.Fatalf("missing raw failure: %#v", response)
	}
}
