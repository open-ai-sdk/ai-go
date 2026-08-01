package kie

import (
	"net/http"
	"time"

	"github.com/open-ai-sdk/ai-go/transport"
)

// Config controls how a Kie.AI Provider talks to the upstream API.
//
// When BaseURL is empty, the SDK targets `https://api.kie.ai` directly. When
// BaseURL is set, upstream paths are mirrored under `{BaseURL}/api/kie/...` —
// see kie-url.go.
type Config struct {
	// APIKey is the Bearer token sent in the Authorization header. When the
	// SDK runs behind a proxy that swaps credentials, this may carry an
	// arbitrary token (e.g. a user JWT) instead of the raw Kie API key.
	APIKey string

	// BaseURL optionally overrides the upstream host with a proxy URL.
	BaseURL string

	// Timeout bounds a single HTTP call (defaults to 120s).
	Timeout time.Duration

	// PollInterval is the wait between status checks (defaults to 1s).
	PollInterval time.Duration

	// PollTimeout is the maximum wall time for Generate to await a
	// terminal task state (defaults to 5 minutes).
	PollTimeout time.Duration

	// Headers are extra HTTP headers added to every request.
	Headers map[string]string

	// HTTPClient lets callers inject a custom client (default: shared client
	// constructed from Timeout).
	HTTPClient *http.Client
}

func newTransportClient(config Config) (*transport.Client, error) {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.kie.ai"
	}
	headers := make(http.Header, len(config.Headers))
	for key, value := range config.Headers {
		headers.Set(key, value)
	}
	return transport.NewClient(transport.ClientConfig{
		BaseURL: baseURL,
		Headers: headers,
		Auth: func(request *http.Request) {
			if config.APIKey != "" {
				request.Header.Set("Authorization", "Bearer "+config.APIKey)
			}
		},
		HTTPClient: config.HTTPClient,
		Provider:   "kie",
	})
}

const (
	defaultTimeout      = 120 * time.Second
	defaultPollInterval = 1 * time.Second
	defaultPollTimeout  = 5 * time.Minute
)

// resolved returns a copy of cfg with non-positive fields filled in with
// defaults. Negative durations are treated as unset (same as zero).
func (c Config) resolved() Config {
	if c.Timeout <= 0 {
		c.Timeout = defaultTimeout
	}
	if c.PollInterval <= 0 {
		c.PollInterval = defaultPollInterval
	}
	if c.PollTimeout <= 0 {
		c.PollTimeout = defaultPollTimeout
	}
	if c.HTTPClient == nil {
		// One-shot POST plus polling GETs (not a stream), so a client-wide
		// timeout is safe here — it bounds each request rather than a stream.
		c.HTTPClient = &http.Client{Timeout: c.Timeout}
	}
	return c
}
