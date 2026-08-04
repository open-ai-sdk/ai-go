package transport

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
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

func TestClient_DoStreamOwnsSuccessfulResponseBody(t *testing.T) {
	body := &closeRecorder{
		ReadCloser: io.NopCloser(strings.NewReader("data: hello\n\n")),
		closed:     make(chan struct{}),
	}
	client, err := NewClient(ClientConfig{
		BaseURL: "https://api.example.test",
		HTTPClient: DoerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := client.NewRequest(context.Background(), http.MethodGet, "stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	events, err := client.DoStream(
		context.Background(),
		req,
		nil,
		func(_ context.Context, reader *SSEReader, out chan<- aikit.StreamEvent) error {
			frame, readErr := reader.Next()
			if readErr != nil {
				return readErr
			}
			out <- aikit.StreamEvent{Type: aikit.StreamEventTextDelta, TextDelta: frame.Data}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var text string
	for event := range events {
		if event.Type == aikit.StreamEventError {
			t.Fatal(event.Error)
		}
		text += event.TextDelta
	}
	if text != "hello" {
		t.Fatalf("stream text = %q, want hello", text)
	}
	select {
	case <-body.closed:
	default:
		t.Fatal("successful streaming response body was not closed")
	}
	if count := body.closes(); count != 1 {
		t.Fatalf("body Close calls = %d, want 1", count)
	}
}

func TestClient_DoStreamClosesErrorResponseBody(t *testing.T) {
	body := &closeRecorder{
		ReadCloser: io.NopCloser(strings.NewReader(`{"error":{"message":"bad"}}`)),
		closed:     make(chan struct{}),
	}
	client, err := NewClient(ClientConfig{
		BaseURL:  "https://api.example.test",
		Provider: "test",
		HTTPClient: DoerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     make(http.Header),
				Body:       body,
			}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := client.NewRequest(context.Background(), http.MethodGet, "stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.DoStream(context.Background(), req, nil, nil); err == nil {
		t.Fatal("expected typed HTTP error")
	}
	select {
	case <-body.closed:
	default:
		t.Fatal("error response body was not closed")
	}
}

func TestClient_DoStreamRejectsMalformedResponses(t *testing.T) {
	tests := []struct {
		name string
		resp *http.Response
	}{
		{name: "nil response", resp: nil},
		{name: "nil body", resp: &http.Response{StatusCode: http.StatusOK}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewClient(ClientConfig{
				BaseURL: "https://api.example.test",
				HTTPClient: DoerFunc(func(*http.Request) (*http.Response, error) {
					return test.resp, nil
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			req, err := client.NewRequest(
				context.Background(),
				http.MethodGet,
				"stream",
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = client.DoStream(context.Background(), req, nil, nil); err == nil {
				t.Fatal("expected malformed response error")
			}
		})
	}
}

func TestClient_DoStreamClosesBodyWhenWrapperReturnsNil(t *testing.T) {
	body := &closeRecorder{
		ReadCloser: io.NopCloser(strings.NewReader("stream")),
		closed:     make(chan struct{}),
	}
	client, err := NewClient(ClientConfig{
		BaseURL: "https://api.example.test",
		HTTPClient: DoerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := client.NewRequest(context.Background(), http.MethodGet, "stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.DoStream(
		context.Background(),
		req,
		func(io.ReadCloser) io.ReadCloser { return nil },
		nil,
	); err == nil {
		t.Fatal("expected nil wrapper result error")
	}
	select {
	case <-body.closed:
	default:
		t.Fatal("original response body was not closed")
	}
}
