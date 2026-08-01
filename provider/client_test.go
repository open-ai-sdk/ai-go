package provider_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/provider"
)

type testPolicy struct {
	baseURL string
	secret  string
}

func (p testPolicy) ProviderName() string { return "test" }
func (p testPolicy) BaseURL() string      { return p.baseURL }
func (p testPolicy) Authorize(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+p.secret)
}

func TestClientAppliesPolicyAndSharedHeaders(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if got := req.Header.Get("Authorization"); got != "Bearer secret-value" {
			t.Errorf("Authorization = %q", got)
		}
		if got := req.Header.Get("X-SDK-Test"); got != "enabled" {
			t.Errorf("X-SDK-Test = %q", got)
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	client, err := provider.NewClient(testPolicy{
		baseURL: server.URL,
		secret:  "secret-value",
	}, provider.ClientConfig{
		Headers: http.Header{"X-SDK-Test": []string{"enabled"}},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	req, err := client.NewRequest(context.Background(), http.MethodGet, "health", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()
}

func TestClientRejectsInvalidEndpoint(t *testing.T) {
	t.Parallel()

	_, err := provider.NewClient(testPolicy{baseURL: "://bad", secret: "secret"}, provider.ClientConfig{})
	if err == nil || !strings.Contains(err.Error(), "configure transport") {
		t.Fatalf("NewClient error = %v", err)
	}
}

func TestClientFormattingRedactsPolicySecrets(t *testing.T) {
	t.Parallel()

	client, err := provider.NewClient(
		testPolicy{baseURL: "https://example.com/v1", secret: "super-secret-value"},
		provider.ClientConfig{},
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	for _, formatted := range []string{
		fmt.Sprintf("%v", client), fmt.Sprintf("%#v", client),
		fmt.Sprintf("%+v", *client), fmt.Sprintf("%#v", *client),
	} {
		if strings.Contains(formatted, "super-secret-value") {
			t.Fatalf("formatted client leaked secret: %s", formatted)
		}
		if strings.Contains(formatted, "example.com") {
			t.Fatalf("formatted client leaked endpoint details: %s", formatted)
		}
	}
}
