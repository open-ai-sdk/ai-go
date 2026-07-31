package openai

import (
	"context"

	"github.com/open-ai-sdk/ai-go/ai"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/provider/internal/openaichat"
)

const chatProviderName = "openai-chat"

// ChatLanguageModel implements ai.LanguageModel for the OpenAI Chat Completions API.
// Use NewChatLanguageModel to construct one.
type ChatLanguageModel struct {
	inner *openaichat.LanguageModel
}

var _ llm.Model = (*ChatLanguageModel)(nil)

// NewChatLanguageModel creates an OpenAI-backed ai.LanguageModel using the
// Chat Completions API (/chat/completions).
//
// This is distinct from NewLanguageModel which targets the Responses API.
// Use Chat Completions when you need:
//   - Broad model compatibility (gpt-3.5-turbo, gpt-4o, gpt-4.1, o-series)
//   - OpenAI-standard request/response shape
//   - Structured output via response_format (gpt-4o-2024-08-06+)
//
// Use NewLanguageModel (Responses API) when you need:
//   - previous_response_id continuation
//   - Built-in web search or file inputs
//   - Responses-native multi-modal output
func NewChatLanguageModel(modelID string, cfg Config) *ChatLanguageModel {
	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	inner := openaichat.NewLanguageModel(openaichat.ModelConfig{
		ModelID:      modelID,
		ProviderName: chatProviderName,
		BaseURL:      base,
		APIKey:       cfg.APIKey,
		Timeout:      cfg.Timeout,
		Capabilities: openaichat.CapabilityFlags{
			SupportsStructuredOutput: true,
			SupportsStreamUsage:      true,
		},
		ChunkTimeout: cfg.ChunkTimeout,
		ExtraBodyFieldsForRequest: func(req llm.Request) map[string]any {
			opts, _ := parseChatProviderOptions(req.ProviderOptions)
			extra := make(map[string]any)
			if opts.ReasoningEffort != "" {
				extra["reasoning_effort"] = opts.ReasoningEffort
			}
			if opts.User != "" {
				extra["user"] = opts.User
			}
			if len(extra) == 0 {
				return nil
			}
			return extra
		},
	})
	return &ChatLanguageModel{inner: inner}
}

// parseChatProviderOptions extracts ChatProviderOptions from a generic provider
// options map. A missing "openai" key returns zero options; a matching invalid
// value returns an error.
func parseChatProviderOptions(opts map[string]any) (ChatProviderOptions, error) {
	if opts == nil {
		return ChatProviderOptions{}, nil
	}
	raw, ok := opts["openai"]
	if !ok {
		return ChatProviderOptions{}, nil
	}
	switch p := raw.(type) {
	case ChatProviderOptions:
		return p, nil
	case *ChatProviderOptions:
		if p == nil {
			return ChatProviderOptions{}, llm.ProviderOptionTypeError("openai", raw)
		}
		return *p, nil
	case reasoningEffortOption:
		return ChatProviderOptions{ReasoningEffort: p.effort}, nil
	case map[string]any:
		var options ChatProviderOptions
		if err := llm.DecodeJSONProviderOptions("openai", p, &options); err != nil {
			return ChatProviderOptions{}, err
		}
		return options, nil
	default:
		return ChatProviderOptions{}, llm.ProviderOptionTypeError("openai", raw)
	}
}

// ModelID returns the OpenAI model identifier.
func (m *ChatLanguageModel) ModelID() string { return m.inner.ModelID() }

// Stream sends a streaming Chat Completions request and returns a channel of
// normalized ai.StreamEvents.
func (m *ChatLanguageModel) Stream(
	ctx context.Context,
	req llm.Request,
) (<-chan ai.StreamEvent, error) {
	if _, err := parseChatProviderOptions(req.ProviderOptions); err != nil {
		return nil, err
	}
	return m.inner.Stream(ctx, req)
}
