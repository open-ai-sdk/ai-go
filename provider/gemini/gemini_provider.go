package gemini

import (
	"os"

	"github.com/open-ai-sdk/ai-go/llm"
)

// Provider constructs Gemini language and image model handles that share one
// immutable configuration.
type Provider struct {
	config Config
}

// NewProvider constructs a Gemini provider.
func NewProvider(config Config) *Provider { return &Provider{config: config} }

// NewFromEnv constructs a Gemini provider using GEMINI_API_KEY.
func NewFromEnv() *Provider {
	return NewProvider(Config{APIKey: os.Getenv("GEMINI_API_KEY")})
}

// Name returns the stable provider registry name.
func (*Provider) Name() string { return "gemini" }

// LanguageModel constructs a Gemini OpenAI-compatible language model.
func (provider *Provider) LanguageModel(modelID string) llm.Model {
	return NewLanguageModel(modelID, provider.config)
}

// ImageModel constructs a Gemini native image model.
func (provider *Provider) ImageModel(modelID string) llm.ImageModel {
	return NewImageModel(modelID, provider.config)
}

var (
	_ llm.LanguageProvider = (*Provider)(nil)
	_ llm.ImageProvider    = (*Provider)(nil)
)
