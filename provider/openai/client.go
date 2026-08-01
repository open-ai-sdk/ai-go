package openai

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/open-ai-sdk/ai-go/provider"
	"github.com/open-ai-sdk/ai-go/provider/openaicompat"
	"github.com/open-ai-sdk/ai-go/transport"
)

const defaultTimeout = 120 * time.Second

type providerPolicy struct {
	apiKey  string
	baseURL string
}

func (p providerPolicy) ProviderName() string { return "openai" }
func (p providerPolicy) BaseURL() string      { return p.baseURL }
func (p providerPolicy) Authorize(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
}

// Client owns OpenAI credentials, endpoints, and reusable HTTP resources.
// Construct one Client and derive lightweight model handles from it.
type Client struct {
	apiKey       string
	baseURL      string
	timeout      time.Duration
	chunkTimeout time.Duration
	streamDoer   transport.Doer
	responses    *provider.Client[providerPolicy]
	uploads      *provider.Client[providerPolicy]
}

// NewClient validates config and constructs a reusable OpenAI client.
func NewClient(cfg Config) (*Client, error) {
	return newClient(cfg, true)
}

func newClient(cfg Config, requireAPIKey bool) (*Client, error) {
	if requireAPIKey && strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("openai: API key is required")
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if requireAPIKey {
		if timeout < 0 {
			return nil, fmt.Errorf("openai: timeout must not be negative")
		}
		if cfg.ChunkTimeout < 0 {
			return nil, fmt.Errorf("openai: chunk timeout must not be negative")
		}
	}

	streamDoer := cfg.HTTPClient
	if streamDoer == nil {
		streamDoer = transport.NewStreamingClient(timeout)
	}
	uploadDoer := cfg.HTTPClient
	if uploadDoer == nil {
		uploadDoer = &http.Client{Timeout: timeout}
	}
	policy := providerPolicy{apiKey: cfg.APIKey, baseURL: baseURL}
	responses, err := provider.NewClient(policy, provider.ClientConfig{
		Headers:    http.Header{"Content-Type": []string{"application/json"}},
		HTTPClient: streamDoer,
	})
	if err != nil {
		return nil, err
	}
	uploads, err := provider.NewClient(policy, provider.ClientConfig{
		HTTPClient:   uploadDoer,
		ProviderName: "openai-file-upload",
	})
	if err != nil {
		return nil, err
	}
	return &Client{
		apiKey:       cfg.APIKey,
		baseURL:      baseURL,
		timeout:      timeout,
		chunkTimeout: cfg.ChunkTimeout,
		streamDoer:   streamDoer,
		responses:    responses,
		uploads:      uploads,
	}, nil
}

// CompletionModel creates a Responses API model handle. Responses may contain
// text, reasoning, tool calls, sources, and generated files/images.
func (c *Client) CompletionModel(modelID string) *LanguageModel {
	return &LanguageModel{modelID: modelID, client: c}
}

// ChatModel creates an OpenAI Chat Completions model handle.
func (c *Client) ChatModel(modelID string) *ChatLanguageModel {
	return openaicompat.NewModel(openaicompat.Config{
		Provider:     chatBackend{baseURL: c.baseURL},
		ModelID:      modelID,
		APIKey:       c.apiKey,
		Timeout:      c.timeout,
		ChunkTimeout: c.chunkTimeout,
		HTTPClient:   c.streamDoer,
	})
}

// String intentionally excludes credentials.
func (Client) String() string { return "openai.Client{}" }

// GoString provides the same credential-safe representation for %#v.
func (c Client) GoString() string { return c.String() }
