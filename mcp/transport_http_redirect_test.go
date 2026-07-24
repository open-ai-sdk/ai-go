package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPTransportRedirectPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/target", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	blocked := NewHTTPTransport(HTTPTransportConfig{URL: server.URL + "/redirect"})
	if _, err := blocked.client.Get(server.URL + "/redirect"); err == nil {
		t.Fatal("default redirect policy must reject redirects")
	}
	following := NewHTTPTransport(HTTPTransportConfig{URL: server.URL, Redirect: RedirectFollow})
	resp, err := following.client.Get(server.URL + "/redirect")
	if err != nil {
		t.Fatalf("follow policy: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}
