package gemini

import (
	"fmt"
	"time"

	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/provider/openaicompat"
	"github.com/open-ai-sdk/ai-go/transport"
)

const defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta/openai"

// Config configures Gemini language, embedding, and image models.
type Config struct {
	APIKey               string
	BaseURL              string
	Timeout              time.Duration
	OutputDimensionality int
	ChunkTimeout         time.Duration
	HTTPClient           transport.Doer
}

// LanguageModel implements basic Gemini chat through its OpenAI-compatible API.
// Use [NewNativeLanguageModel] for grounding, citations, or multimodal output.
type LanguageModel = openaicompat.Model

type compatBackend struct{ baseURL string }

func (b compatBackend) BaseURL() string { return b.baseURL }
func (compatBackend) AuthHeader(key string) (string, string) {
	return "Authorization", "Bearer " + key
}
func (compatBackend) ProviderName() string { return "gemini" }
func (compatBackend) Capabilities() openaicompat.CapabilityFlags {
	return openaicompat.CapabilityFlags{
		SupportsStructuredOutput: true,
		SupportsStreamUsage:      true,
	}
}

func (compatBackend) SanitizeTools(tools []map[string]any) []map[string]any {
	return sanitizeToolSchemas(tools)
}

func (compatBackend) RewriteRequest(
	req llm.Request,
	body map[string]any,
) (map[string]any, error) {
	options, err := resolveProviderOptions(req.ProviderOptions)
	if err != nil {
		return nil, err
	}
	if options.EnableGoogleSearch ||
		len(options.ResponseModalities) > 0 ||
		options.ImageConfig != nil {
		return nil, fmt.Errorf(
			"gemini: grounding and multimodal output require NewNativeLanguageModel",
		)
	}
	if options.ThinkingConfig == nil {
		return body, nil
	}
	thinking := buildCompatThinkingConfig(options.ThinkingConfig)
	if len(thinking) == 0 {
		return body, nil
	}
	body["extra_body"] = map[string]any{
		"google": map[string]any{"thinking_config": thinking},
	}
	return body, nil
}

func buildCompatThinkingConfig(config *ThinkingConfig) map[string]any {
	thinking := make(map[string]any)
	if config.ThinkingBudget != nil {
		thinking["thinking_budget"] = *config.ThinkingBudget
	}
	if config.IncludeThoughts != nil {
		thinking["include_thoughts"] = *config.IncludeThoughts
	}
	if config.ThinkingLevel != "" {
		thinking["thinking_level"] = config.ThinkingLevel
	}
	return thinking
}

// NewLanguageModel creates a basic OpenAI-compatible Gemini model.
func NewLanguageModel(modelID string, config Config) *LanguageModel {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return openaicompat.NewModel(openaicompat.Config{
		Provider: compatBackend{baseURL: baseURL}, ModelID: modelID,
		APIKey: config.APIKey, Timeout: config.Timeout,
		ChunkTimeout: config.ChunkTimeout, HTTPClient: config.HTTPClient,
	})
}
