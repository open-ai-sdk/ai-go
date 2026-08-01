package openai

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

const defaultBaseURL = "https://api.openai.com/v1"

// LanguageModel implements aikit.LanguageModel for the OpenAI Responses API.
type LanguageModel struct {
	modelID      string
	chunkTimeout time.Duration
	client       *transport.Client
	clientErr    error
	uploadClient *transport.Client
	uploadErr    error
}

var _ llm.Model = (*LanguageModel)(nil)

// Config holds options for constructing an OpenAI LanguageModel.
type Config struct {
	APIKey       string
	BaseURL      string        // optional; defaults to https://api.openai.com/v1
	Timeout      time.Duration // optional; defaults to 120s
	ChunkTimeout time.Duration // optional; per-chunk SSE read timeout (0 = disabled)
	HTTPClient   transport.Doer
}

// NewLanguageModel creates an OpenAI-backed aikit.LanguageModel using the Responses API.
func NewLanguageModel(modelID string, cfg Config) *LanguageModel {
	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = transport.NewStreamingClient(timeout)
	}
	client, clientErr := transport.NewClient(transport.ClientConfig{
		BaseURL: base,
		Headers: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Auth: func(req *http.Request) {
			req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		},
		HTTPClient: httpClient,
		Provider:   "openai",
	})
	uploadHTTPClient := cfg.HTTPClient
	if uploadHTTPClient == nil {
		uploadHTTPClient = &http.Client{Timeout: timeout}
	}
	uploadClient, uploadErr := transport.NewClient(transport.ClientConfig{
		BaseURL: base,
		Auth: func(req *http.Request) {
			req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		},
		HTTPClient: uploadHTTPClient,
		Provider:   "openai-file-upload",
	})
	return &LanguageModel{
		modelID:      modelID,
		chunkTimeout: cfg.ChunkTimeout,
		client:       client,
		clientErr:    clientErr,
		uploadClient: uploadClient,
		uploadErr:    uploadErr,
	}
}

// ModelID returns the OpenAI model identifier.
func (m *LanguageModel) ModelID() string { return m.modelID }

// Stream sends a streaming Responses API request and returns a channel of
// normalized aikit.StreamEvents. Warnings from request encoding are emitted as
// the first event when non-empty.
func (m *LanguageModel) Stream(ctx context.Context, req llm.Request) (<-chan aikit.StreamEvent, error) {
	apiReq, warnings, err := encodeRequest(m.modelID, req, true)
	if err != nil {
		return nil, fmt.Errorf("openai: encode request: %w", err)
	}

	resp, err := m.doRequest(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	body := resp.Body
	if m.chunkTimeout > 0 {
		body = transport.NewTimeoutReader(resp.Body, m.chunkTimeout)
	}

	resp.Body = body
	return transport.Stream(
		ctx,
		resp,
		func(
			ctx context.Context,
			reader *transport.SSEReader,
			events chan<- aikit.StreamEvent,
		) error {
			return decodeResponsesSSEStream(
				ctx,
				reader,
				events,
				warnings...,
			)
		},
	), nil
}

// doRequest marshals body, sends POST /responses, and returns the HTTP response.
// Non-2xx responses become a typed *APIError; the body is parsed for code/message
// then discarded, never embedded.
func (m *LanguageModel) doRequest(ctx context.Context, apiReq responsesRequest) (*http.Response, error) {
	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	if m.clientErr != nil {
		return nil, fmt.Errorf("openai: configure transport: %w", m.clientErr)
	}
	httpReq, err := m.client.NewRequest(
		ctx,
		http.MethodPost,
		"responses",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("openai: build http request: %w", err)
	}
	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: http request: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Typed error carrying status/code/message/request-ID/Retry-After; the
		// raw body is parsed then discarded, never embedded.
		return nil, transport.APIErrorFromResponse(ctx, "openai", resp)
	}
	return resp, nil
}
