package transport

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func TestErrorBodyCaptureIsOptInAndNeverPrinted(t *testing.T) {
	body := `{"error":{"code":"bad_request","message":"invalid"},"diagnostic":"secret-marker"}`
	for _, capture := range []bool{false, true} {
		ctx := context.Background()
		if capture {
			ctx = WithErrorBodyCapture(ctx)
		}
		resp := &http.Response{StatusCode: 400, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
		err := APIErrorFromResponse(ctx, "example", resp)
		if got := string(err.RawBody); capture && got != body || !capture && got != "" {
			t.Fatalf("capture=%v body=%q", capture, got)
		}
		if strings.Contains(err.Error(), "secret-marker") {
			t.Fatal("diagnostic body appeared in Error")
		}
	}
}

func TestResponseObserversComposeAndOwnHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Request-Id", "req-neutral")
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()
	client, err := NewClient(ClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	var order []int
	ctx := WithResponseObserver(context.Background(), func(meta ResponseMetadata) {
		order = append(order, 1)
		meta.Header.Set("X-Request-Id", "mutated")
	})
	ctx = WithResponseObserver(ctx, func(meta ResponseMetadata) {
		order = append(order, 2)
		if meta.StatusCode != 200 || meta.Header.Get("X-Request-Id") != "req-neutral" {
			t.Errorf("metadata=%+v", meta)
		}
	})
	req, err := client.NewRequest(ctx, http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if len(order) != 2 || order[0] != 1 || order[1] != 2 || resp.Header.Get("X-Request-Id") != "req-neutral" {
		t.Fatalf("order=%v headers=%v", order, resp.Header)
	}
}

func TestStatuslessStreamRetryClassification(t *testing.T) {
	for _, tc := range []struct {
		code string
		want bool
	}{{"server_error", true}, {"invalid_request_error", false}, {"", false}} {
		if got := (&aikit.APIError{Code: tc.code}).Retryable(); got != tc.want {
			t.Errorf("code=%q retryable=%v", tc.code, got)
		}
	}
}
