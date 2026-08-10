package anthropic

import "testing"

func TestProviderNameAndEnvironment(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "environment-key")
	provider := NewFromEnv()
	if provider.Name() != "anthropic" {
		t.Fatalf("Name() = %q, want anthropic", provider.Name())
	}
	if provider.config.APIKey != "environment-key" {
		t.Fatalf("APIKey = %q", provider.config.APIKey)
	}
	if model := provider.LanguageModel("claude-test"); model.ModelID() != "claude-test" {
		t.Fatalf("LanguageModel().ModelID() = %q", model.ModelID())
	}
}
