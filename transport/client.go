package transport

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// AuthFunc applies provider authentication to an outgoing request.
type AuthFunc func(*http.Request)

// ClientConfig configures a shared provider HTTP client.
type ClientConfig struct {
	BaseURL    string
	Headers    http.Header
	Auth       AuthFunc
	HTTPClient Doer
	Provider   string
}

// Client builds provider requests around a base URL, common headers, an auth
// hook, and an injectable HTTP backend.
type Client struct {
	baseURL  *url.URL
	headers  http.Header
	auth     AuthFunc
	doer     Doer
	provider string
}

// NewClient creates a shared provider client.
func NewClient(config ClientConfig) (*Client, error) {
	baseURL, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("transport: parse base URL: %w", err)
	}
	if baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("transport: base URL must be absolute")
	}
	doer := config.HTTPClient
	if doer == nil {
		doer = http.DefaultClient
	}
	headers := config.Headers.Clone()
	if headers == nil {
		headers = make(http.Header)
	}
	return &Client{
		baseURL:  baseURL,
		headers:  headers,
		auth:     config.Auth,
		doer:     doer,
		provider: config.Provider,
	}, nil
}

// NewRequest builds a request relative to the client's base URL. An absolute
// same-origin target is accepted; cross-origin targets are rejected so common
// authentication headers cannot leak to an unrelated host.
func (c *Client) NewRequest(
	ctx context.Context,
	method string,
	target string,
	body io.Reader,
) (*http.Request, error) {
	requestURL, err := c.resolve(target)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, fmt.Errorf("transport: build request: %w", err)
	}
	req.Header = c.headers.Clone()
	if c.auth != nil {
		c.auth(req)
	}
	return req, nil
}

// Do executes req through the injected backend.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	resp, err := c.doer.Do(req)
	if err != nil {
		if c.provider == "" {
			return nil, err
		}
		return nil, aikitTransportError(c.provider, err)
	}
	return resp, nil
}

// DoStream executes req and transfers successful response-body ownership to
// the returned event stream. The body is closed when decoding completes,
// fails, panics, or the context is cancelled. Non-2xx responses are converted
// to a typed API error before this method returns.
func (c *Client) DoStream(
	ctx context.Context,
	req *http.Request,
	wrapBody func(io.ReadCloser) io.ReadCloser,
	decode StreamDecoder,
) (<-chan aikit.StreamEvent, error) {
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("transport: HTTP response is nil")
	}
	if resp.Body == nil {
		return nil, fmt.Errorf("transport: HTTP response has no body")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, APIErrorFromResponse(ctx, c.provider, resp)
	}
	body := resp.Body
	if wrapBody != nil {
		body = wrapBody(body)
		if body == nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("transport: response body wrapper returned nil")
		}
	}

	out := make(chan aikit.StreamEvent, defaultStreamBuffer)
	raw := make(chan aikit.StreamEvent, defaultStreamBuffer)
	go func() {
		runDecoderBody(ctx, body, decode, raw)
	}()
	go relayStream(ctx, raw, out)
	return out, nil
}

func (c *Client) resolve(target string) (string, error) {
	ref, err := url.Parse(target)
	if err != nil {
		return "", fmt.Errorf("transport: parse request target: %w", err)
	}
	if ref.Host != "" && !ref.IsAbs() {
		return "", fmt.Errorf(
			"transport: protocol-relative request target is not allowed",
		)
	}
	if ref.IsAbs() {
		if !sameOrigin(c.baseURL, ref) {
			return "", fmt.Errorf(
				"transport: cross-origin request target is not allowed",
			)
		}
		return ref.String(), nil
	}

	base := *c.baseURL
	if !strings.HasSuffix(base.Path, "/") {
		base.Path += "/"
	}
	ref.Path = strings.TrimPrefix(ref.Path, "/")
	resolved := base.ResolveReference(ref)
	if !sameOrigin(c.baseURL, resolved) {
		return "", fmt.Errorf(
			"transport: resolved request target changed origin",
		)
	}
	return resolved.String(), nil
}

func sameOrigin(left, right *url.URL) bool {
	return left.Scheme == right.Scheme &&
		strings.EqualFold(left.Host, right.Host)
}

// DefaultStreamingTransport returns an HTTP transport with bounded connection
// and response-header setup, but no whole-response deadline.
func DefaultStreamingTransport(headerTimeout time.Duration) *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: headerTimeout,
	}
}

// NewStreamingClient returns an HTTP client suitable for long-lived response
// bodies. Its Timeout is intentionally zero; the request context owns the
// overall stream lifetime.
func NewStreamingClient(headerTimeout time.Duration) *http.Client {
	return &http.Client{Transport: DefaultStreamingTransport(headerTimeout)}
}
