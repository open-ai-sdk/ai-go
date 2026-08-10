package ai

import (
	"context"
	"errors"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

type registryLanguageModel struct{ id string }

func (model registryLanguageModel) ModelID() string { return model.id }
func (registryLanguageModel) Stream(context.Context, llm.Request) (<-chan aikit.StreamEvent, error) {
	events := make(chan aikit.StreamEvent)
	close(events)
	return events, nil
}

type registryImageModel struct{ id string }

func (model registryImageModel) ModelID() string { return model.id }
func (registryImageModel) Generate(context.Context, llm.GenerateImageRequest) (*llm.GenerateImageResult, error) {
	return &llm.GenerateImageResult{}, nil
}

type registryFullProvider struct{ name string }

func (provider registryFullProvider) Name() string { return provider.name }
func (registryFullProvider) LanguageModel(id string) llm.Model {
	return registryLanguageModel{id: id}
}

func (registryFullProvider) ImageModel(id string) llm.ImageModel {
	return registryImageModel{id: id}
}

type registryLanguageProvider struct{ name string }

func (provider registryLanguageProvider) Name() string { return provider.name }
func (registryLanguageProvider) LanguageModel(id string) llm.Model {
	return registryLanguageModel{id: id}
}

func TestRegistryResolvesCapabilities(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(registryFullProvider{name: "test"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	language, err := registry.LanguageModel("test", "language-id")
	if err != nil || language.ModelID() != "language-id" {
		t.Fatalf("LanguageModel() = %#v, %v", language, err)
	}
	image, err := registry.ImageModel("test", "image-id")
	if err != nil || image.ModelID() != "image-id" {
		t.Fatalf("ImageModel() = %#v, %v", image, err)
	}
}

func TestRegistryRejectsDuplicateAndUnknownProviders(t *testing.T) {
	var registry Registry
	if err := registry.Register(registryFullProvider{name: "test"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registry.Register(registryFullProvider{name: "test"}); !errors.Is(err, ErrProviderRegistered) {
		t.Fatalf("duplicate Register() error = %v", err)
	}
	if _, err := registry.LanguageModel("missing", "model"); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("unknown LanguageModel() error = %v", err)
	}
}

func TestRegistryReportsCapabilityMiss(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(registryLanguageProvider{name: "language-only"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if _, err := registry.ImageModel("language-only", "image"); !errors.Is(err, ErrProviderCapability) {
		t.Fatalf("ImageModel() error = %v", err)
	}
}

func TestRegistryRejectsInvalidProviders(t *testing.T) {
	registry := NewRegistry()
	var typedNil *registryFullProvider
	for _, provider := range []llm.Provider{nil, typedNil, registryFullProvider{}} {
		if err := registry.Register(provider); !errors.Is(err, ErrInvalidProvider) {
			t.Fatalf("Register(%#v) error = %v", provider, err)
		}
	}
}
