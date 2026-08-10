package openai

import (
	"os"

	"github.com/open-ai-sdk/ai-go/llm"
)

// Provider constructs OpenAI language and image model handles from one
// immutable configuration.
type Provider struct {
	config Config
}

// NewProvider constructs an OpenAI provider.
func NewProvider(config Config) *Provider { return &Provider{config: config} }

// NewFromEnv constructs an OpenAI provider using OPENAI_API_KEY.
func NewFromEnv() *Provider {
	return NewProvider(Config{APIKey: os.Getenv("OPENAI_API_KEY")})
}

// Name returns the stable provider registry name.
func (*Provider) Name() string { return "openai" }

// LanguageModel constructs an OpenAI Responses API model.
func (provider *Provider) LanguageModel(modelID string) llm.Model {
	return NewLanguageModel(modelID, provider.config)
}

// ImageModel constructs an OpenAI Images API model.
func (provider *Provider) ImageModel(modelID string) llm.ImageModel {
	return NewImageModel(modelID, provider.config)
}

var (
	_ llm.LanguageProvider = (*Provider)(nil)
	_ llm.ImageProvider    = (*Provider)(nil)
)
