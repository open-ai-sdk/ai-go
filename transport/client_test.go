package transport

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestClient_BuildsRequestAndUsesInjectedDoer(t *testing.T) {
	var received *http.Request
	client, err := NewClient(ClientConfig{
		BaseURL: "https://api.example.test/v1",
		Headers: http.Header{"X-Common": []string{"shared"}},
		Auth: func(req *http.Request) {
			req.Header.Set("Authorization", "Bearer token")
		},
		HTTPClient: DoerFunc(func(req *http.Request) (*http.Response, error) {
			received = req
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("{}")),
				Header:     make(http.Header),
			}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	req, err := client.NewRequest(
		context.Background(),
		http.MethodPost,
		"responses",
		strings.NewReader("{}"),
	)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if received.URL.String() != "https://api.example.test/v1/responses" {
		t.Errorf("URL = %q", received.URL)
	}
	if received.Header.Get("X-Common") != "shared" {
		t.Errorf("common header = %q", received.Header.Get("X-Common"))
	}
	if received.Header.Get("Authorization") != "Bearer token" {
		t.Errorf("authorization = %q", received.Header.Get("Authorization"))
	}
}

func TestClient_RejectsCrossOriginTargetBeforeApplyingAuth(t *testing.T) {
	authCalled := false
	client, err := NewClient(ClientConfig{
		BaseURL: "https://api.example.test/v1",
		Auth: func(*http.Request) {
			authCalled = true
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.NewRequest(
		context.Background(),
		http.MethodGet,
		"https://attacker.example/file",
		nil,
	)
	if err == nil {
		t.Fatal("expected cross-origin target rejection")
	}
	if authCalled {
		t.Fatal("auth hook ran for rejected cross-origin target")
	}
}

func TestClient_RejectsProtocolRelativeTargetBeforeApplyingAuth(t *testing.T) {
	authCalled := false
	client, err := NewClient(ClientConfig{
		BaseURL: "https://api.example.test/v1",
		Auth: func(*http.Request) {
			authCalled = true
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.NewRequest(
		context.Background(),
		http.MethodGet,
		"//attacker.example/file",
		nil,
	)
	if err == nil {
		t.Fatal("expected protocol-relative target rejection")
	}
	if authCalled {
		t.Fatal("auth hook ran for rejected protocol-relative target")
	}
}

func TestClient_ProviderWrappedNetworkErrorRemainsRetryable(t *testing.T) {
	client, err := NewClient(ClientConfig{
		BaseURL:  "https://api.example.test",
		Provider: "test-provider",
		HTTPClient: DoerFunc(func(*http.Request) (*http.Response, error) {
			return nil, io.ErrUnexpectedEOF
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := client.NewRequest(
		context.Background(),
		http.MethodPost,
		"responses",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	if !IsRetryable(err) {
		t.Fatalf("wrapped network error = %v, want retryable", err)
	}
}
