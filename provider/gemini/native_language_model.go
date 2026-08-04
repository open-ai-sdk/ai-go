package gemini

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

const nativeBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// NativeLanguageModel implements aikit.LanguageModel using the native Gemini API
// (:streamGenerateContent endpoint). Unlike the OpenAI-compatible LanguageModel,
// this provider fully supports Google Search grounding, native thinking config,
// and other Gemini-only features that are unavailable via the OpenAI compatibility
// layer.
//
// Use NewNativeLanguageModel to construct an instance.
type NativeLanguageModel struct {
	modelID      string
	cfg          Config
	chunkTimeout time.Duration
	client       *transport.Client
	clientErr    error
}

var _ llm.Model = (*NativeLanguageModel)(nil)

// NewNativeLanguageModel creates a Gemini-backed aikit.LanguageModel that uses the
// native Gemini API directly (not the OpenAI-compatible endpoint).
//
// Use this when you need features like Google Search grounding or native thinking
// configuration. For basic chat completions, NewLanguageModel (OpenAI-compatible)
// may also work.
func NewNativeLanguageModel(modelID string, cfg Config) *NativeLanguageModel {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	cfg.Timeout = timeout
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = nativeBaseURL
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = transport.NewStreamingClient(timeout)
	}
	client, clientErr := transport.NewClient(transport.ClientConfig{
		BaseURL: baseURL,
		Headers: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Auth: func(req *http.Request) {
			req.Header.Set("x-goog-api-key", cfg.APIKey)
		},
		HTTPClient: httpClient,
		Provider:   "gemini-native",
	})
	return &NativeLanguageModel{
		modelID:      modelID,
		cfg:          cfg,
		chunkTimeout: cfg.ChunkTimeout,
		client:       client,
		clientErr:    clientErr,
	}
}

// ModelID returns the Gemini model identifier.
func (m *NativeLanguageModel) ModelID() string { return m.modelID }

// NativeSchemaSupport is separate from generic tool support because Gemini's
// native API rejects response schemas together with function declarations.
func (*NativeLanguageModel) NativeSchemaSupport() llm.NativeSchemaSupport {
	return llm.NativeSchemaSuppressesTools
}

// Stream sends a streaming request to the native Gemini API and returns a
// channel of normalized aikit.StreamEvents.
func (m *NativeLanguageModel) Stream(ctx context.Context, req llm.Request) (<-chan aikit.StreamEvent, error) {
	if _, err := resolveProviderOptions(req.ProviderOptions); err != nil {
		return nil, err
	}
	// Build native request body.
	nr := encodeNativeRequestForModel(m.modelID, req)

	// Encode tools + toolConfig.
	opts := parseProviderOptions(req.ProviderOptions)
	toolResult := encodeNativeTools(req.Tools, req.ToolChoice, opts)
	nr.Tools = toolResult.Tools
	nr.ToolConfig = toolResult.ToolConfig

	body, err := json.Marshal(nr)
	if err != nil {
		return nil, fmt.Errorf("gemini-native: marshal request: %w", err)
	}

	if m.clientErr != nil {
		return nil, fmt.Errorf(
			"gemini-native: configure transport: %w",
			m.clientErr,
		)
	}
	target := fmt.Sprintf(
		"models/%s:streamGenerateContent?alt=sse",
		m.modelID,
	)
	httpReq, err := m.client.NewRequest(
		ctx,
		http.MethodPost,
		target,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("gemini-native: build http request: %w", err)
	}
	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini-native: http request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// Typed error carrying status/code/message/request-ID/Retry-After; the
		// raw body is parsed then discarded, never embedded.
		return nil, transport.APIErrorFromResponse(ctx, "gemini-native", resp)
	}

	if m.chunkTimeout > 0 {
		resp.Body = transport.NewTimeoutReader(resp.Body, m.chunkTimeout)
	}
	return transport.Stream(ctx, resp, decodeNativeSSEStream), nil
}
