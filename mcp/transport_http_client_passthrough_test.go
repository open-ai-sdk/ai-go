package mcp

import (
	"errors"
	"net/http"
	"testing"
)

// TestHTTPTransport_CallerHTTPClientPassthrough verifies that a caller-supplied
// *http.Client is used unchanged and its redirect policy is preserved, even when
// the config's Redirect policy is not RedirectFollow. The transport must only
// impose its own CheckRedirect when it builds the client itself.
func TestHTTPTransport_CallerHTTPClientPassthrough(t *testing.T) {
	callerPolicy := errors.New("caller redirect policy")
	custom := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return callerPolicy },
	}

	tr := NewHTTPTransport(HTTPTransportConfig{
		URL:        "http://example.test",
		HTTPClient: custom,
		Redirect:   RedirectError, // non-follow: transport must still not override the caller
	})

	if tr.client != custom {
		t.Fatal("expected caller-supplied HTTPClient to be used as-is")
	}
	if tr.client.CheckRedirect == nil {
		t.Fatal("caller CheckRedirect was cleared")
	}
	if err := tr.client.CheckRedirect(nil, nil); !errors.Is(err, callerPolicy) {
		t.Errorf("caller redirect policy overridden: got %v, want %v", err, callerPolicy)
	}
}

// TestHTTPTransport_DefaultClientBlocksRedirects verifies that when no client is
// supplied and the policy is not RedirectFollow, the transport-built client
// blocks redirects.
func TestHTTPTransport_DefaultClientBlocksRedirects(t *testing.T) {
	tr := NewHTTPTransport(HTTPTransportConfig{URL: "http://example.test", Redirect: RedirectError})
	if tr.client.CheckRedirect == nil {
		t.Fatal("expected transport-built client to block redirects")
	}
}
