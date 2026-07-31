package openai_compatible

import (
	"time"

	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/provider/openaicompat"
	"github.com/open-ai-sdk/ai-go/transport"
)

type Config struct {
	APIKey, BaseURL, ProviderName string
	Timeout, ChunkTimeout         time.Duration
	Headers                       map[string]string
	SupportsStructuredOutput      bool
	SupportsStreamUsage           bool
	TransformRequest              func(map[string]any) map[string]any
	HTTPClient                    transport.Doer
}

type LanguageModel = openaicompat.Model

type backend struct{ config Config }

func (b backend) BaseURL() string { return b.config.BaseURL }
func (b backend) AuthHeader(key string) (string, string) {
	return "Authorization", "Bearer " + key
}

func (b backend) ProviderName() string {
	if b.config.ProviderName != "" {
		return b.config.ProviderName
	}
	return "openaiCompatible"
}

func (b backend) Capabilities() openaicompat.CapabilityFlags {
	return openaicompat.CapabilityFlags{
		SupportsStructuredOutput: b.config.SupportsStructuredOutput,
		SupportsStreamUsage:      b.config.SupportsStreamUsage,
	}
}

func (b backend) RewriteRequest(_ llm.Request, body map[string]any) (map[string]any, error) {
	if b.config.TransformRequest != nil {
		body = b.config.TransformRequest(body)
	}
	return body, nil
}

func NewLanguageModel(modelID string, config Config) *LanguageModel {
	return openaicompat.NewModel(openaicompat.Config{
		Provider: backend{config}, ModelID: modelID, APIKey: config.APIKey,
		Headers: config.Headers, Timeout: config.Timeout,
		ChunkTimeout: config.ChunkTimeout, HTTPClient: config.HTTPClient,
	})
}
