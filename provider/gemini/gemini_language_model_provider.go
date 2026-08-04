package gemini

import (
	"fmt"
	"strings"
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

type compatBackend struct {
	baseURL string
	modelID string
}

func (b compatBackend) BaseURL() string { return b.baseURL }
func (compatBackend) AuthHeader(key string) (string, string) {
	return "Authorization", "Bearer " + key
}
func (compatBackend) ProviderName() string { return "gemini" }
func (b compatBackend) Capabilities() openaicompat.CapabilityFlags {
	return openaicompat.CapabilityFlags{
		SupportsStructuredOutput: true,
		NativeSchema:             geminiNativeSchemaSupport(b.modelID),
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
		return sanitizeCompatOutputSchema(body), nil
	}
	thinking := buildCompatThinkingConfig(options.ThinkingConfig)
	if len(thinking) == 0 {
		return sanitizeCompatOutputSchema(body), nil
	}
	body["extra_body"] = map[string]any{
		"google": map[string]any{"thinking_config": thinking},
	}
	return sanitizeCompatOutputSchema(body), nil
}

func sanitizeCompatOutputSchema(body map[string]any) map[string]any {
	format, ok := body["response_format"].(map[string]any)
	if !ok {
		return body
	}
	jsonSchema, ok := format["json_schema"].(map[string]any)
	if !ok {
		return body
	}
	if schema, ok := jsonSchema["schema"].(map[string]any); ok {
		jsonSchema["schema"] = sanitizeMap(schema)
	}
	return body
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
		Provider: compatBackend{baseURL: baseURL, modelID: modelID}, ModelID: modelID,
		APIKey: config.APIKey, Timeout: config.Timeout,
		ChunkTimeout: config.ChunkTimeout, HTTPClient: config.HTTPClient,
	})
}

func geminiNativeSchemaSupport(modelID string) llm.NativeSchemaSupport {
	modelID = strings.ToLower(modelID)
	if modelID == "gemini-3" || strings.HasPrefix(modelID, "gemini-3-") {
		return llm.NativeSchemaFull
	}
	return llm.NativeSchemaSuppressesTools
}
