package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/transport"
)

// Policy defines provider-wide endpoint and authentication behavior.
// Implementations should be small immutable values. Client deliberately does
// not format a Policy, because policies commonly contain credentials.
type Policy interface {
	ProviderName() string
	BaseURL() string
	Authorize(*http.Request)
}

// ClientConfig configures a generic provider client. BaseURL overrides the
// policy default when non-empty.
type ClientConfig struct {
	BaseURL      string
	Headers      http.Header
	HTTPClient   transport.Doer
	ProviderName string
}

// Client combines provider policy with shared HTTP request behavior. Concrete
// provider clients compose one or more Client values and expose only the model
// capabilities they actually implement.
type Client[P Policy] struct {
	policy    P
	baseURL   string
	transport *transport.Client
}

// NewClient constructs a reusable provider client and validates its endpoint.
func NewClient[P Policy](policy P, config ClientConfig) (*Client[P], error) {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = policy.BaseURL()
	}
	providerName := config.ProviderName
	if providerName == "" {
		providerName = policy.ProviderName()
	}
	client, err := transport.NewClient(transport.ClientConfig{
		BaseURL:    baseURL,
		Headers:    config.Headers,
		Auth:       policy.Authorize,
		HTTPClient: config.HTTPClient,
		Provider:   providerName,
	})
	if err != nil {
		return nil, fmt.Errorf("provider %s: configure transport: %w", policy.ProviderName(), err)
	}
	return &Client[P]{policy: policy, baseURL: baseURL, transport: client}, nil
}

// ProviderName returns the stable provider identifier.
func (c *Client[P]) ProviderName() string { return c.policy.ProviderName() }

// BaseURL returns the configured provider endpoint.
func (c *Client[P]) BaseURL() string { return c.baseURL }

// NewRequest builds an authenticated request relative to the provider base URL.
func (c *Client[P]) NewRequest(
	ctx context.Context,
	method string,
	target string,
	body io.Reader,
) (*http.Request, error) {
	return c.transport.NewRequest(ctx, method, target, body)
}

// Do executes a regular HTTP request.
func (c *Client[P]) Do(req *http.Request) (*http.Response, error) {
	return c.transport.Do(req)
}

// DoStream executes an SSE request and transfers response-body ownership to
// the returned stream.
func (c *Client[P]) DoStream(
	ctx context.Context,
	req *http.Request,
	wrapBody func(io.ReadCloser) io.ReadCloser,
	decode transport.StreamDecoder,
) (<-chan aikit.StreamEvent, error) {
	return c.transport.DoStream(ctx, req, wrapBody, decode)
}

// String intentionally excludes the policy value because it may contain an
// API key or other credentials.
func (c Client[P]) String() string {
	return fmt.Sprintf("provider.Client{name:%q}", c.ProviderName())
}

// GoString provides the same credential-safe representation for %#v.
func (c Client[P]) GoString() string { return c.String() }
