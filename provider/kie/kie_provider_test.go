package kie

import "testing"

func TestProviderRegistryMethods(t *testing.T) {
	t.Setenv("KIE_API_KEY", "env-key")
	provider := NewFromEnv()
	if provider.Name() != "kie" {
		t.Fatalf("Name() = %q, want kie", provider.Name())
	}
	if provider.Config().APIKey != "env-key" {
		t.Fatalf("NewFromEnv API key was not loaded")
	}
	model := provider.ImageModel(ModelSeedreamV4TextToImage.String())
	if model.ModelID() != ModelSeedreamV4TextToImage.String() {
		t.Fatalf("ImageModel ModelID() = %q", model.ModelID())
	}
}

func TestImageCompatibilityConstructorRemainsTyped(t *testing.T) {
	provider := NewProvider("test-key")
	model := provider.Image(ModelSeedreamV4Edit)
	if model.ModelID() != ModelSeedreamV4Edit.String() {
		t.Fatalf("Image() ModelID() = %q", model.ModelID())
	}
}
