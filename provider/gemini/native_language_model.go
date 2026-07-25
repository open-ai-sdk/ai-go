package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/open-ai-sdk/ai-go/ai"
	"github.com/open-ai-sdk/ai-go/httputil"
	"github.com/open-ai-sdk/ai-go/internal/safego"
)

const nativeBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// NativeLanguageModel implements ai.LanguageModel using the native Gemini API
// (:streamGenerateContent endpoint). Unlike the OpenAI-compatible LanguageModel,
// this provider fully supports Google Search grounding, native thinking config,
// and other Gemini-only features that are unavailable via the OpenAI compatibility
// layer.
//
// Use NewNativeLanguageModel to construct an instance.
type NativeLanguageModel struct {
	modelID string
	cfg     Config
	client  *http.Client
}

// NewNativeLanguageModel creates a Gemini-backed ai.LanguageModel that uses the
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
	return &NativeLanguageModel{
		modelID: modelID,
		cfg:     cfg,
		// Streaming path: no client-wide timeout (it would cap the whole SSE
		// exchange); cfg.Timeout becomes a response-header deadline instead.
		client: httputil.NewStreamingClient(timeout),
	}
}

// ModelID returns the Gemini model identifier.
func (m *NativeLanguageModel) ModelID() string { return m.modelID }

// Stream sends a streaming request to the native Gemini API and returns a
// channel of normalized ai.StreamEvents.
func (m *NativeLanguageModel) Stream(ctx context.Context, req ai.LanguageModelRequest) (<-chan ai.StreamEvent, error) {
	// Build native request body.
	nr := encodeNativeRequest(req)

	// Encode tools + toolConfig.
	opts := parseProviderOptions(req.ProviderOptions)
	toolResult := encodeNativeTools(req.Tools, req.ToolChoice, opts)
	nr.Tools = toolResult.Tools
	nr.ToolConfig = toolResult.ToolConfig

	// Generate warnings for unsupported option combinations.
	warnings := warningsForRequest(m.modelID, req)

	body, err := json.Marshal(nr)
	if err != nil {
		return nil, fmt.Errorf("gemini-native: marshal request: %w", err)
	}

	baseURL := m.cfg.BaseURL
	if baseURL == "" {
		baseURL = nativeBaseURL
	}
	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse", baseURL, m.modelID)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini-native: build http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", m.cfg.APIKey)

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini-native: http request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// Typed error carrying status/code/message/request-ID/Retry-After; the
		// raw body is parsed then discarded, never embedded.
		return nil, httputil.APIErrorFromResponse(ctx, "gemini-native", resp)
	}

	raw := make(chan ai.StreamEvent, 64)
	go func() {
		defer resp.Body.Close()
		decodeNativeSSEStream(ctx, resp.Body, raw)
	}()

	if len(warnings) == 0 {
		return httputil.GuardStream(ctx, raw), nil
	}

	// Wrap the channel to inject warnings into the first finish event. Sends are
	// ctx-guarded and the upstream is drained on cancel, matching the Stream
	// context contract.
	out := make(chan ai.StreamEvent, 64)
	go func() {
		defer close(out)
		// A panic while injecting warnings surfaces as an error event
		// (ctx-guarded) before close instead of crashing the process.
		defer safego.Recover(nil, func(err error) {
			select {
			case out <- ai.StreamEvent{Type: ai.StreamEventError, Error: err}:
			case <-ctx.Done():
			}
		})
		finishInjected := false
		for ev := range raw {
			if !finishInjected && ev.Type == ai.StreamEventFinish {
				ev.Warnings = append(warnings, ev.Warnings...)
				finishInjected = true
			}
			select {
			case out <- ev:
			case <-ctx.Done():
				for range raw {
				}
				return
			}
		}
	}()
	return out, nil
}
