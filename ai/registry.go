package ai

import "github.com/open-ai-sdk/ai-go/llm"

var (
	// ErrInvalidProvider reports a nil provider or an empty provider name.
	ErrInvalidProvider = llm.ErrInvalidProvider
	// ErrProviderRegistered reports an attempt to replace an existing provider.
	ErrProviderRegistered = llm.ErrProviderRegistered
	// ErrProviderNotFound reports a lookup for an unknown provider name.
	ErrProviderNotFound = llm.ErrProviderNotFound
	// ErrProviderCapability reports that a provider lacks a requested capability.
	ErrProviderCapability = llm.ErrProviderCapability
)

// NewRegistry constructs an empty provider registry.
func NewRegistry() *llm.Registry { return llm.NewRegistry() }
