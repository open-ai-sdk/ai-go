package llm

// Provider identifies a model provider by its stable registry name.
//
// Capability-specific provider interfaces embed Provider. Implementations
// expose only the capabilities they support.
type Provider interface {
	Name() string
}

// LanguageProvider constructs language-model handles for a provider.
type LanguageProvider interface {
	Provider
	LanguageModel(modelID string) Model
}

// ImageProvider constructs image-model handles for a provider.
type ImageProvider interface {
	Provider
	ImageModel(modelID string) ImageModel
}
