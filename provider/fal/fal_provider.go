package fal

import (
	"os"

	"github.com/open-ai-sdk/ai-go/llm"
)

// Provider is the entry point for constructing fal models.
//
// Construct with NewProvider; reuse across goroutines (HTTP client is shared).
type Provider struct {
	cfg Config
}

// Option mutates a Config during NewProvider construction.
type Option func(*Config)

// Name returns the stable provider registry key.
func (*Provider) Name() string { return "fal" }

// WithBaseURL routes queue traffic to another origin, e.g. a proxy mirror.
func WithBaseURL(baseURL string) Option {
	return func(c *Config) { c.BaseURL = baseURL }
}

// WithConfig replaces the entire Config; later Options still apply.
func WithConfig(cfg Config) Option {
	return func(c *Config) { *c = cfg }
}

// NewProvider builds a fal provider. apiKey takes precedence over any key set
// by Options; if still empty after Options, the constructor falls back to the
// FAL_KEY environment variable.
func NewProvider(apiKey string, opts ...Option) *Provider {
	var cfg Config
	for _, opt := range opts {
		opt(&cfg)
	}
	if apiKey != "" {
		cfg.APIKey = apiKey
	}
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("FAL_KEY")
	}
	return &Provider{cfg: cfg.resolved()}
}

// NewFromEnv builds a fal provider using FAL_KEY.
func NewFromEnv(opts ...Option) *Provider { return NewProvider("", opts...) }

// Video constructs a fal video model for a queue endpoint id such as
// "fal-ai/kling-video/o3/pro/image-to-video". Ids need not be known to the
// SDK; an invalid id is reported by every call on the returned model.
func (p *Provider) Video(endpointID string) *VideoModel {
	return newVideoModel(endpointID, p.cfg)
}

// VideoModel constructs a video model by endpoint id for use with ai.Registry.
func (p *Provider) VideoModel(endpointID string) llm.VideoModel {
	return p.Video(endpointID)
}

// Config returns a copy of the resolved configuration so callers cannot
// mutate provider internals (Headers map and HTTPClient are cloned).
func (p *Provider) Config() Config {
	cp := p.cfg
	if p.cfg.Headers != nil {
		cp.Headers = make(map[string]string, len(p.cfg.Headers))
		for k, v := range p.cfg.Headers {
			cp.Headers[k] = v
		}
	}
	if p.cfg.HTTPClient != nil {
		clone := *p.cfg.HTTPClient
		cp.HTTPClient = &clone
	}
	return cp
}

var _ llm.VideoProvider = (*Provider)(nil)
