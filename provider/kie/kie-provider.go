package kie

import "os"

// Provider is the entry point for constructing Kie.AI image models and
// invoking file-upload helpers.
//
// Construct with NewProvider; reuse across goroutines (HTTP client is shared).
type Provider struct {
	cfg Config
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

// NewProvider builds a Kie.AI provider. apiKey takes precedence; if empty, the
// constructor falls back to the KIE_API_KEY environment variable.
func NewProvider(apiKey string, opts ...Option) *Provider {
	cfg := Config{APIKey: apiKey}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("KIE_API_KEY")
	}
	return &Provider{cfg: cfg.resolved()}
}

// Image constructs an ai.ImageModel for the given Kie model ID.
func (p *Provider) Image(modelID ImageModelID) *ImageModel {
	return newImageModel(modelID, p.cfg)
}

// Config returns a copy of the resolved configuration.
func (p *Provider) Config() Config { return p.cfg }
