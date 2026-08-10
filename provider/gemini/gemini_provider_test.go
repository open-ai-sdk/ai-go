package gemini

import (
	"testing"

	"github.com/open-ai-sdk/ai-go/llm"
)

func TestProviderCapabilities(t *testing.T) {
	provider := NewProvider(Config{APIKey: "test-key"})
	if provider.Name() != "gemini" {
		t.Fatalf("Name() = %q, want gemini", provider.Name())
	}
	if model := provider.LanguageModel("language"); model.ModelID() != "language" {
		t.Fatalf("LanguageModel().ModelID() = %q", model.ModelID())
	}
	if model := provider.ImageModel("image"); model.ModelID() != "image" {
		t.Fatalf("ImageModel().ModelID() = %q", model.ModelID())
	}

	var _ llm.LanguageProvider = provider
	var _ llm.ImageProvider = provider
}

func TestNewFromEnv(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "environment-key")
	provider := NewFromEnv()
	if provider.config.APIKey != "environment-key" {
		t.Fatalf("APIKey = %q", provider.config.APIKey)
	}
}
