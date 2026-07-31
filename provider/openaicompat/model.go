package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/transport"
)

// Config holds call-independent configuration for a compatible model.
type Config struct {
	// Provider supplies the endpoint, authentication, and optional behavior
	// hooks. A nil provider is reported when Stream is called.
	Provider Compat
	// ModelID is the model identifier sent to the API.
	ModelID string
	// APIKey is used for Authorization: Bearer <key>.
	APIKey string
	// Headers holds additional HTTP headers to include on every request.
	Headers map[string]string
	// Timeout is the HTTP client timeout. Defaults to 120s.
	Timeout time.Duration
	// ChunkTimeout is the per-chunk SSE read timeout. Each received chunk resets
	// the timer. If no data arrives within this duration, the stream is aborted
	// with ErrChunkTimeout. Zero means no per-chunk timeout.
	ChunkTimeout time.Duration
	// HTTPClient is an optional custom doer. When non-nil, Timeout is ignored
	// and the caller owns transport + timeout. Used for injecting logging
	// RoundTrippers or proxy behavior without changing this package.
	HTTPClient transport.Doer
}

// Model implements [llm.Model] using OpenAI-style Chat Completions.
type Model struct {
	cfg       Config
	client    *transport.Client
	clientErr error
}

var _ llm.Model = (*Model)(nil)

// NewModel creates a compatible model with the given configuration.
func NewModel(cfg Config) *Model {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		timeout := cfg.Timeout
		if timeout == 0 {
			timeout = 120 * time.Second
		}
		// Streaming path: no client-wide timeout (it would cap the whole SSE
		// exchange); cfg.Timeout becomes a response-header deadline instead.
		httpClient = transport.NewStreamingClient(timeout)
	}
	headers := make(http.Header, len(cfg.Headers)+1)
	headers.Set("Content-Type", "application/json")
	for key, value := range cfg.Headers {
		headers.Set(key, value)
	}
	if cfg.Provider == nil {
		return &Model{
			cfg:       cfg,
			clientErr: fmt.Errorf("openaicompat: provider is required"),
		}
	}
	authName, authValue := cfg.Provider.AuthHeader(cfg.APIKey)
	client, clientErr := transport.NewClient(transport.ClientConfig{
		BaseURL: cfg.Provider.BaseURL(),
		Headers: headers,
		Auth: func(req *http.Request) {
			if authName != "" {
				req.Header.Set(authName, authValue)
			}
		},
		HTTPClient: httpClient,
		Provider:   providerName(cfg.Provider),
	})
	return &Model{
		cfg:       cfg,
		client:    client,
		clientErr: clientErr,
	}
}

// ModelID returns the configured model identifier.
func (m *Model) ModelID() string { return m.cfg.ModelID }

// Stream sends a streaming request and returns normalized events.
func (m *Model) Stream(
	ctx context.Context,
	req llm.Request,
) (<-chan aikit.StreamEvent, error) {
	name := providerName(m.cfg.Provider)
	capabilities := providerCapabilities(m.cfg.Provider)
	params := EncodeRequestParams{
		ModelID:                  m.cfg.ModelID,
		SupportsStructuredOutput: capabilities.SupportsStructuredOutput,
		IncludeStreamUsage:       capabilities.SupportsStreamUsage,
	}
	if sanitizer, ok := m.cfg.Provider.(ToolSanitizer); ok {
		params.SanitizeTools = sanitizer.SanitizeTools
	}

	cr, encodeWarnings, err := EncodeRequest(params, req, true)
	if err != nil {
		return nil, fmt.Errorf("%s: encode request: %w", name, err)
	}

	var body []byte
	if rewriter, ok := m.cfg.Provider.(RequestRewriter); ok {
		rawMap, mapErr := structToMap(cr)
		if mapErr != nil {
			return nil, fmt.Errorf("%s: marshal request to map: %w", name, mapErr)
		}
		rawMap, err = rewriter.RewriteRequest(req, rawMap)
		if err != nil {
			return nil, fmt.Errorf("%s: rewrite request: %w", name, err)
		}
		body, err = json.Marshal(rawMap)
	} else {
		body, err = json.Marshal(cr)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: marshal request: %w", name, err)
	}

	if m.clientErr != nil {
		return nil, fmt.Errorf("%s: configure transport: %w", name, m.clientErr)
	}
	httpReq, err := m.client.NewRequest(
		ctx, http.MethodPost,
		"chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("%s: build http request: %w", name, err)
	}
	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s: http request: %w", name, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, transport.APIErrorFromResponse(ctx, name, resp)
	}

	respBody := resp.Body
	if m.cfg.ChunkTimeout > 0 {
		respBody = transport.NewTimeoutReader(resp.Body, m.cfg.ChunkTimeout)
	}

	resp.Body = respBody
	decodeParams := SSEDecodeParams{ProviderName: name}
	if configurator, ok := m.cfg.Provider.(DecodeConfigurator); ok {
		decodeParams = configurator.DecodeParams()
		if decodeParams.ProviderName == "" {
			decodeParams.ProviderName = name
		}
	}
	decodeParams.EncodeWarnings = append(
		decodeParams.EncodeWarnings,
		encodeWarnings...,
	)
	return transport.Stream(
		ctx,
		resp,
		func(
			ctx context.Context,
			reader *transport.SSEReader,
			events chan<- aikit.StreamEvent,
		) error {
			return DecodeSSEStream(ctx, reader, events, decodeParams)
		},
	), nil
}

func providerName(provider Compat) string {
	if named, ok := provider.(Named); ok && named.ProviderName() != "" {
		return named.ProviderName()
	}
	return "openai-compatible"
}

func providerCapabilities(provider Compat) CapabilityFlags {
	if capable, ok := provider.(CapabilityProvider); ok {
		return capable.Capabilities()
	}
	return CapabilityFlags{}
}

// structToMap marshals v to JSON and unmarshals into a map[string]any.
func structToMap(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}
