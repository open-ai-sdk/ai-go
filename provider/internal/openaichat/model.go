package openaichat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/open-ai-sdk/ai-go/ai"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/transport"
)

// ModelConfig holds all configuration for the shared chat completions LanguageModel.
type ModelConfig struct {
	// ModelID is the model identifier sent to the API.
	ModelID string
	// ProviderName is used in error messages and metadata (e.g. "gemini", "openai").
	ProviderName string
	// BaseURL is the API endpoint base (e.g. "https://api.openai.com/v1").
	BaseURL string
	// APIKey is used for Authorization: Bearer <key>.
	APIKey string
	// Headers holds additional HTTP headers to include on every request.
	Headers map[string]string
	// Timeout is the HTTP client timeout. Defaults to 120s.
	Timeout time.Duration
	// Capabilities declares optional feature support.
	Capabilities CapabilityFlags
	// SanitizeTools is an optional hook to clean tool schemas before sending.
	SanitizeTools func(tools []map[string]any) []map[string]any
	// TransformRequestBody is an optional hook to mutate the request body map before
	// sending. The map is a JSON-serializable representation of ChatRequest.
	// Extra keys added here are preserved in the outgoing request body.
	TransformRequestBody func(body map[string]any) map[string]any
	// ExtraToolsForRequest is an optional hook to supply additional provider-specific
	// tool entries per request (e.g. {"type": "google_search"} for Gemini grounding).
	// Called with the current ai.LanguageModelRequest; may return nil.
	ExtraToolsForRequest func(req llm.Request) []map[string]any
	// ExtraBodyFieldsForRequest is an optional hook to supply additional top-level
	// request body fields per request (e.g. thinkingConfig for Gemini).
	// Returned map keys are merged into the JSON body before sending.
	ExtraBodyFieldsForRequest func(req llm.Request) map[string]any
	// MetadataExtractor is an optional hook to extract provider metadata from SSE chunks.
	MetadataExtractor func(chunk StreamChunk) map[string]any
	// ChunkTimeout is the per-chunk SSE read timeout. Each received chunk resets
	// the timer. If no data arrives within this duration, the stream is aborted
	// with ErrChunkTimeout. Zero means no per-chunk timeout.
	ChunkTimeout time.Duration
	// HTTPClient is an optional custom client. When non-nil, Timeout is ignored
	// and the caller owns transport + timeout. Used for injecting logging
	// RoundTrippers or proxy behavior without changing this package.
	HTTPClient *http.Client
}

// LanguageModel implements ai.LanguageModel using OpenAI-style chat completions.
type LanguageModel struct {
	cfg       ModelConfig
	client    *transport.Client
	clientErr error
}

var _ llm.Model = (*LanguageModel)(nil)

// NewLanguageModel creates a LanguageModel with the given configuration.
func NewLanguageModel(cfg ModelConfig) *LanguageModel {
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
	client, clientErr := transport.NewClient(transport.ClientConfig{
		BaseURL: cfg.BaseURL,
		Headers: headers,
		Auth: func(req *http.Request) {
			req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		},
		HTTPClient: httpClient,
	})
	return &LanguageModel{
		cfg:       cfg,
		client:    client,
		clientErr: clientErr,
	}
}

// ModelID returns the configured model identifier.
func (m *LanguageModel) ModelID() string { return m.cfg.ModelID }

// Stream sends a streaming chat request and returns a channel of normalized ai.StreamEvents.
func (m *LanguageModel) Stream(ctx context.Context, req llm.Request) (<-chan ai.StreamEvent, error) {
	params := EncodeRequestParams{
		ModelID:            m.cfg.ModelID,
		SanitizeTools:      m.cfg.SanitizeTools,
		IncludeStreamUsage: m.cfg.Capabilities.SupportsStreamUsage,
	}
	if m.cfg.ExtraToolsForRequest != nil {
		params.ExtraTools = m.cfg.ExtraToolsForRequest(req)
	}

	cr, encodeWarnings, err := EncodeRequest(params, req, true)
	if err != nil {
		return nil, fmt.Errorf("%s: encode request: %w", m.cfg.ProviderName, err)
	}

	// Merge provider-specific extra body fields (e.g. thinkingConfig).
	var extraFields map[string]any
	if m.cfg.ExtraBodyFieldsForRequest != nil {
		extraFields = m.cfg.ExtraBodyFieldsForRequest(req)
	}

	var body []byte
	if extraFields != nil || m.cfg.TransformRequestBody != nil {
		// Marshal to map so extra fields and transforms can be merged.
		rawMap, mapErr := structToMap(cr)
		if mapErr != nil {
			return nil, fmt.Errorf("%s: marshal request to map: %w", m.cfg.ProviderName, mapErr)
		}
		for k, v := range extraFields {
			rawMap[k] = v
		}
		if m.cfg.TransformRequestBody != nil {
			rawMap = m.cfg.TransformRequestBody(rawMap)
		}
		body, err = json.Marshal(rawMap)
	} else {
		body, err = json.Marshal(cr)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: marshal request: %w", m.cfg.ProviderName, err)
	}

	if m.clientErr != nil {
		return nil, fmt.Errorf(
			"%s: configure transport: %w",
			m.cfg.ProviderName,
			m.clientErr,
		)
	}
	httpReq, err := m.client.NewRequest(
		ctx, http.MethodPost,
		"chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("%s: build http request: %w", m.cfg.ProviderName, err)
	}
	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s: http request: %w", m.cfg.ProviderName, err)
	}
	if resp.StatusCode != http.StatusOK {
		// Typed error carrying status/code/message/request-ID/Retry-After; the
		// raw body is parsed then discarded, never embedded.
		return nil, transport.APIErrorFromResponse(ctx, m.cfg.ProviderName, resp)
	}

	respBody := resp.Body
	if m.cfg.ChunkTimeout > 0 {
		respBody = NewTimeoutReader(resp.Body, m.cfg.ChunkTimeout)
	}

	resp.Body = respBody
	return transport.Stream(
		ctx,
		resp,
		func(
			ctx context.Context,
			reader *transport.SSEReader,
			events chan<- ai.StreamEvent,
		) error {
			return DecodeSSEStream(
				ctx,
				reader,
				events,
				SSEDecodeParams{
					ProviderName:      m.cfg.ProviderName,
					MetadataExtractor: m.cfg.MetadataExtractor,
					EncodeWarnings:    encodeWarnings,
				},
			)
		},
	), nil
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
