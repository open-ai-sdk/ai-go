package openai

import (
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/provider/openaicompat"
)

const chatProviderName = "openai-chat"

// ChatLanguageModel implements [llm.Model] for Chat Completions.
type ChatLanguageModel = openaicompat.Model

type chatBackend struct{ baseURL string }

func (b chatBackend) BaseURL() string { return b.baseURL }
func (chatBackend) AuthHeader(key string) (string, string) {
	return "Authorization", "Bearer " + key
}
func (chatBackend) ProviderName() string { return chatProviderName }
func (chatBackend) Capabilities() openaicompat.CapabilityFlags {
	return openaicompat.CapabilityFlags{
		SupportsStructuredOutput: true,
		SupportsStreamUsage:      true,
	}
}

func (chatBackend) RewriteRequest(
	req llm.Request,
	body map[string]any,
) (map[string]any, error) {
	opts, err := parseChatProviderOptions(req.ProviderOptions)
	if err != nil {
		return nil, err
	}
	if opts.ReasoningEffort != "" {
		body["reasoning_effort"] = opts.ReasoningEffort
	}
	if opts.User != "" {
		body["user"] = opts.User
	}
	if opts.StrictJSONSchema != nil {
		if format, ok := body["response_format"].(map[string]any); ok {
			if schema, ok := format["json_schema"].(map[string]any); ok {
				schema["strict"] = *opts.StrictJSONSchema
			}
		}
	}
	return body, nil
}

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
	client, err := newClient(cfg, false)
	if err != nil {
		// Preserve the legacy constructor's deferred-error behavior by using the
		// existing compatible model constructor for invalid configuration.
		base := cfg.BaseURL
		if base == "" {
			base = defaultBaseURL
		}
		return openaicompat.NewModel(openaicompat.Config{
			Provider: chatBackend{baseURL: base}, ModelID: modelID,
			APIKey: cfg.APIKey, Timeout: cfg.Timeout,
			ChunkTimeout: cfg.ChunkTimeout, HTTPClient: cfg.HTTPClient,
		})
	}
	return client.ChatModel(modelID)
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
