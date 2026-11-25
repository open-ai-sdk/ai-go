package fal

import (
	"net/http"
	"time"

	"github.com/open-ai-sdk/ai-go/transport"
)

// Config controls how a fal Provider talks to the queue API.
type Config struct {
	// APIKey is sent as `Authorization: Key <APIKey>`. When the SDK runs
	// behind a proxy that swaps credentials, this may carry another token.
	APIKey string

	// BaseURL overrides the queue host (defaults to https://queue.fal.run).
	// Endpoint paths are appended beneath it, preserving any base path.
	BaseURL string

	// Timeout bounds a single HTTP call (defaults to 120s).
	Timeout time.Duration

	// MaxReplyBytes bounds raw queue replies; zero leaves replies unbounded.
	MaxReplyBytes int64
	// RequireUTF8Reply rejects non-UTF-8 raw queue reply bodies.
	RequireUTF8Reply bool
	// DisableRedirects returns redirect responses without following them,
	// including through an injected HTTPClient (which is copied, not mutated).
	DisableRedirects bool

	// Headers are extra HTTP headers added to every request.
	Headers map[string]string

	// HTTPClient lets callers inject a custom client (default: a client
	// constructed from Timeout).
	HTTPClient *http.Client

	// PollInterval is the wait between status checks (defaults to 2s).
	PollInterval time.Duration

	// PollTimeout is the maximum wall time for GenerateVideo to await a
	// completed request (defaults to 10 minutes).
	PollTimeout time.Duration
}

const (
	defaultBaseURL      = "https://queue.fal.run"
	defaultTimeout      = 120 * time.Second
	defaultPollInterval = 2 * time.Second
	defaultPollTimeout  = 10 * time.Minute
)

// resolved returns a copy of c with non-positive fields filled in with
// defaults. Negative durations are treated as unset (same as zero).
func (c Config) resolved() Config {
	if c.BaseURL == "" {
		c.BaseURL = defaultBaseURL
	}
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
		// timeout bounds each request.
		c.HTTPClient = &http.Client{Timeout: c.Timeout}
	} else if c.DisableRedirects {
		clone := *c.HTTPClient
		c.HTTPClient = &clone
	}
	if c.DisableRedirects {
		c.HTTPClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	return c
}

func newTransportClient(config Config) (*transport.Client, error) {
	headers := make(http.Header, len(config.Headers))
	for key, value := range config.Headers {
		headers.Set(key, value)
	}
	return transport.NewClient(transport.ClientConfig{
		BaseURL: config.BaseURL,
		Headers: headers,
		Auth: func(request *http.Request) {
			if config.APIKey != "" {
				request.Header.Set("Authorization", "Key "+config.APIKey)
			}
		},
		HTTPClient: config.HTTPClient,
		Provider:   "fal",
	})
}
