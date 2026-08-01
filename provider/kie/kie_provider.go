package kie

import (
	"os"

	"github.com/open-ai-sdk/ai-go/transport"
)

// Provider is the entry point for constructing Kie.AI image models and
// invoking file-upload helpers.
//
// Construct with NewProvider; reuse across goroutines (HTTP client is shared).
type Provider struct {
	cfg       Config
	client    *transport.Client
	clientErr error
}

// Option mutates a Config during NewProvider construction.
type Option func(*Config)

// WithBaseURL routes traffic through a proxy mirror.
// Pass the proxy origin with no trailing slash, e.g. "https://gen.example.com".
func WithBaseURL(baseURL string) Option {
	return func(c *Config) { c.BaseURL = baseURL }
}

// WithConfig replaces the entire Config; later Options still apply.
func WithConfig(cfg Config) Option {
	return func(c *Config) { *c = cfg }
}

// NewProvider builds a Kie.AI provider. apiKey takes precedence over any key
// set by Options; if still empty after Options, the constructor falls back to
// the KIE_API_KEY environment variable.
func NewProvider(apiKey string, opts ...Option) *Provider {
	var cfg Config
	for _, opt := range opts {
		opt(&cfg)
	}
	// Explicit apiKey parameter always wins over whatever Options may have set.
	if apiKey != "" {
		cfg.APIKey = apiKey
	}
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("KIE_API_KEY")
	}
	cfg = cfg.resolved()
	client, clientErr := newTransportClient(cfg)
	return &Provider{cfg: cfg, client: client, clientErr: clientErr}
}

// Image constructs an ai.ImageModel for the given Kie model ID.
func (p *Provider) Image(modelID ImageModelID) *ImageModel {
	return newImageModel(modelID, p.cfg)
}

// Config returns a deep copy of the resolved configuration so callers cannot
// mutate provider internals (Headers map and HTTPClient are cloned).
func (p *Provider) Config() Config {
	cp := p.cfg

	// Deep-clone the mutable Headers map.
	if p.cfg.Headers != nil {
		cp.Headers = make(map[string]string, len(p.cfg.Headers))
		for k, v := range p.cfg.Headers {
			cp.Headers[k] = v
		}
	}

	// Deep-clone the HTTPClient pointer by copying the struct.
	if p.cfg.HTTPClient != nil {
		clone := *p.cfg.HTTPClient
		cp.HTTPClient = &clone
	}

	return cp
}
