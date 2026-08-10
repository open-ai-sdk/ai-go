package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/transport"
)

const defaultBaseURL = "https://api.openai.com/v1"

// LanguageModel implements aikit.LanguageModel for the OpenAI Responses API.
type LanguageModel struct {
	modelID   string
	client    *Client
	clientErr error
}

var _ llm.Model = (*LanguageModel)(nil)

// Config holds options for constructing an OpenAI Client or model.
type Config struct {
	APIKey       string
	BaseURL      string        // optional; defaults to https://api.openai.com/v1
	Timeout      time.Duration // optional; defaults to 120s
	ChunkTimeout time.Duration // optional; per-chunk SSE read timeout (0 = disabled)
	HTTPClient   transport.Doer
	// ImageResponseMaxBytes bounds a successful Images API response body. Zero
	// defaults to 64 MiB; image requests use a five-minute HTTP timeout unless
	// Timeout or HTTPClient is explicitly configured.
	ImageResponseMaxBytes int64
}

// NewLanguageModel creates an OpenAI-backed aikit.LanguageModel using the Responses API.
func NewLanguageModel(modelID string, cfg Config) *LanguageModel {
	client, err := newClient(cfg, false)
	if err != nil {
		return &LanguageModel{modelID: modelID, clientErr: err}
	}
	return client.CompletionModel(modelID)
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

	httpReq, err := m.buildRequest(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	var wrapBody func(io.ReadCloser) io.ReadCloser
	if m.client.chunkTimeout > 0 {
		wrapBody = func(body io.ReadCloser) io.ReadCloser {
			return transport.NewTimeoutReader(body, m.client.chunkTimeout)
		}
	}
	return m.client.responses.DoStream(
		ctx,
		httpReq,
		wrapBody,
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
	)
}

// buildRequest marshals body and builds POST /responses. The shared transport
// executes it and owns the response lifecycle.
func (m *LanguageModel) buildRequest(ctx context.Context, apiReq responsesRequest) (*http.Request, error) {
	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	if m.clientErr != nil {
		return nil, fmt.Errorf("openai: configure transport: %w", m.clientErr)
	}
	httpReq, err := m.client.responses.NewRequest(
		ctx,
		http.MethodPost,
		"responses",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("openai: build http request: %w", err)
	}
	return httpReq, nil
}
