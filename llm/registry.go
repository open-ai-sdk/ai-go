package llm

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

var (
	// ErrInvalidProvider reports a nil provider or an empty provider name.
	ErrInvalidProvider = errors.New("ai: invalid provider")
	// ErrProviderRegistered reports an attempt to replace an existing provider.
	ErrProviderRegistered = errors.New("ai: provider already registered")
	// ErrProviderNotFound reports a lookup for an unknown provider name.
	ErrProviderNotFound = errors.New("ai: provider not found")
	// ErrProviderCapability reports that a provider lacks a requested capability.
	ErrProviderCapability = errors.New("ai: provider capability unavailable")
)

// Registry resolves provider and model IDs to capability-specific model
// handles. Its zero value is ready for use.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry constructs an empty provider registry.
func NewRegistry() *Registry { return &Registry{} }

// Register adds provider under its stable Name. Re-registering a name returns
// an error rather than silently changing future model resolution.
func (registry *Registry) Register(provider Provider) error {
	if registry == nil || isNilProvider(provider) {
		return ErrInvalidProvider
	}
	name := provider.Name()
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: name is empty", ErrInvalidProvider)
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.providers == nil {
		registry.providers = make(map[string]Provider)
	}
	if _, exists := registry.providers[name]; exists {
		return fmt.Errorf("%w: %q", ErrProviderRegistered, name)
	}
	registry.providers[name] = provider
	return nil
}

// LanguageModel resolves a language model by provider and model ID.
func (registry *Registry) LanguageModel(providerName, modelID string) (Model, error) {
	provider, err := registry.provider(providerName)
	if err != nil {
		return nil, err
	}
	capability, ok := provider.(LanguageProvider)
	if !ok {
		return nil, fmt.Errorf("%w: provider %q does not support language models", ErrProviderCapability, providerName)
	}
	model := capability.LanguageModel(modelID)
	if isNilProvider(model) {
		return nil, fmt.Errorf("%w: provider %q returned a nil language model", ErrProviderCapability, providerName)
	}
	return model, nil
}

// ImageModel resolves an image model by provider and model ID.
func (registry *Registry) ImageModel(providerName, modelID string) (ImageModel, error) {
	provider, err := registry.provider(providerName)
	if err != nil {
		return nil, err
	}
	capability, ok := provider.(ImageProvider)
	if !ok {
		return nil, fmt.Errorf("%w: provider %q does not support image models", ErrProviderCapability, providerName)
	}
	model := capability.ImageModel(modelID)
	if isNilProvider(model) {
		return nil, fmt.Errorf("%w: provider %q returned a nil image model", ErrProviderCapability, providerName)
	}
	return model, nil
}

func (registry *Registry) provider(name string) (Provider, error) {
	if registry == nil {
		return nil, fmt.Errorf("%w: %q", ErrProviderNotFound, name)
	}
	registry.mu.RLock()
	provider := registry.providers[name]
	registry.mu.RUnlock()
	if isNilProvider(provider) {
		return nil, fmt.Errorf("%w: %q", ErrProviderNotFound, name)
	}
	return provider, nil
}

func isNilProvider(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
		reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
