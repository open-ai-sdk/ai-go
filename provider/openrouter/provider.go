package openrouter

import (
	"os"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/provider/openai"
	"github.com/open-ai-sdk/ai-go/provider/openaicompat"
	"github.com/open-ai-sdk/ai-go/transport"
)

// DefaultBaseURL is OpenRouter's API root.
const DefaultBaseURL = "https://openrouter.ai/api/v1"

const providerName = "openrouter"

// Config configures an OpenRouter language model.
type Config struct {
	APIKey string
	// BaseURL overrides DefaultBaseURL.
	BaseURL string
	// Referer and Title are optional app-attribution headers
	// (HTTP-Referer and X-Title).
	Referer string
	Title   string
	// Headers are extra headers sent on every request.
	Headers map[string]string
	// Timeout bounds waiting for response headers. Zero uses the
	// openaicompat default.
	Timeout time.Duration
	// ChunkTimeout aborts a stream after this long without any bytes. SSE
	// keepalive comments count as bytes; use llm.WithStallTimeout to bound
	// time between decoded events instead.
	ChunkTimeout time.Duration
	HTTPClient   transport.Doer
}

// LanguageModel is an OpenRouter chat model.
type LanguageModel = openaicompat.Model

type backend struct{ baseURL string }

func (b backend) BaseURL() string { return b.baseURL }
func (backend) AuthHeader(key string) (string, string) {
	return "Authorization", "Bearer " + key
}
func (backend) ProviderName() string { return providerName }
func (backend) Capabilities() openaicompat.CapabilityFlags {
	return openaicompat.CapabilityFlags{
		SupportsStructuredOutput: true,
		NativeSchema:             llm.NativeSchemaFull,
		SupportsStreamUsage:      true,
	}
}

// RewriteRequest injects the OpenRouter-only request fields.
func (backend) RewriteRequest(req llm.Request, body map[string]any) (map[string]any, error) {
	opts, err := parseProviderOptions(req.ProviderOptions)
	if err != nil {
		return nil, err
	}
	include := opts.IncludeUsage == nil || *opts.IncludeUsage
	if include {
		body["usage"] = map[string]any{"include": true}
	}
	if opts.Reasoning != nil {
		body["reasoning"] = opts.Reasoning
	}
	if opts.Provider != nil {
		body["provider"] = opts.Provider
	}
	if len(opts.Models) > 0 {
		body["models"] = opts.Models
	}
	if len(opts.Transforms) > 0 {
		body["transforms"] = opts.Transforms
	}
	if opts.User != "" {
		body["user"] = opts.User
	}
	if opts.ParallelToolCalls != nil {
		if tools, ok := body["tools"].([]any); ok && len(tools) > 0 {
			body["parallel_tool_calls"] = *opts.ParallelToolCalls
		}
	}
	return body, nil
}

// NewLanguageModel creates an OpenRouter model for modelID (for example
// "openai/gpt-4o-mini").
func NewLanguageModel(modelID string, cfg Config) *LanguageModel {
	base := cfg.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	headers := make(map[string]string, len(cfg.Headers)+2)
	for k, v := range cfg.Headers {
		headers[k] = v
	}
	if cfg.Referer != "" {
		headers["HTTP-Referer"] = cfg.Referer
	}
	if cfg.Title != "" {
		headers["X-Title"] = cfg.Title
	}
	return openaicompat.NewModel(openaicompat.Config{
		Provider: backend{baseURL: base}, ModelID: modelID, APIKey: cfg.APIKey,
		Headers: headers, Timeout: cfg.Timeout, ChunkTimeout: cfg.ChunkTimeout,
		HTTPClient: cfg.HTTPClient,
	})
}

// ResponsesModel is an OpenRouter Responses API model. Request options use
// openai.ProviderOptions; Chat Completions routing options do not apply.
type ResponsesModel = openai.LanguageModel

// NewResponsesModel constructs a Responses API model with OpenRouter endpoints
// and attribution headers, using the OpenAI Responses encoder and decoder.
func NewResponsesModel(modelID string, cfg Config) *ResponsesModel {
	base := cfg.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	headers := make(map[string]string, len(cfg.Headers)+2)
	for name, value := range cfg.Headers {
		headers[name] = value
	}
	if cfg.Referer != "" {
		headers["HTTP-Referer"] = cfg.Referer
	}
	if cfg.Title != "" {
		headers["X-Title"] = cfg.Title
	}
	return openai.NewLanguageModel(modelID, openai.Config{
		APIKey: cfg.APIKey, BaseURL: base, Headers: headers,
		Timeout: cfg.Timeout, ChunkTimeout: cfg.ChunkTimeout, HTTPClient: cfg.HTTPClient,
	})
}

// Provider constructs OpenRouter model handles from one configuration.
type Provider struct{ config Config }

// NewProvider constructs an OpenRouter provider.
func NewProvider(config Config) *Provider { return &Provider{config: config} }

// NewFromEnv constructs a provider using OPENROUTER_API_KEY.
func NewFromEnv() *Provider {
	return NewProvider(Config{APIKey: os.Getenv("OPENROUTER_API_KEY")})
}

// Name returns the stable registry name.
func (*Provider) Name() string { return providerName }

// LanguageModel constructs a model handle for modelID.
func (p *Provider) LanguageModel(modelID string) llm.Model {
	return NewLanguageModel(modelID, p.config)
}

// ResponsesModel constructs an opt-in Responses API model handle.
func (p *Provider) ResponsesModel(modelID string) *ResponsesModel {
	return NewResponsesModel(modelID, p.config)
}

var _ llm.LanguageProvider = (*Provider)(nil)

// Cost returns the cost OpenRouter reported in the usage object (usage.cost,
// in credits/USD as billed by OpenRouter). It is the provider-reported
// figure, not a computed price. ok is false when the usage carries no cost.
func Cost(u aikit.Usage) (cost float64, ok bool) {
	switch v := u.Raw["cost"].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	}
	return 0, false
}
