package openai

import "github.com/open-ai-sdk/ai-go/llm"

// ImageOptions holds OpenAI Images API options passed through
// llm.GenerateImageRequest.ProviderOptions["openai"].
type ImageOptions struct {
	// Quality is one of auto, low, medium, or high.
	Quality string `json:"quality"`
	// Background is one of auto, opaque, or transparent.
	Background string `json:"background"`
	// OutputFormat is one of png, jpeg, or webp.
	OutputFormat string `json:"outputFormat"`
	// OutputCompression is a percentage from 0 to 100. It is supported only
	// for jpeg and webp output.
	OutputCompression *int `json:"outputCompression"`
	// Moderation is one of auto or low.
	Moderation string `json:"moderation"`
	// User is a stable end-user identifier for abuse monitoring.
	User string `json:"user"`
	// InputFidelity is one of low or high and applies to edits.
	InputFidelity string `json:"inputFidelity"`
	// Mask optionally supplies an inline mask for an edit. URL-only masks are
	// rejected so this package never fetches caller-controlled URLs.
	Mask *llm.ImageInput `json:"mask"`
}

// ProviderName identifies the provider-options key.
func (ImageOptions) ProviderName() string { return "openai" }

func parseImageOptions(options map[string]any) (ImageOptions, error) {
	if options == nil {
		return ImageOptions{}, nil
	}
	raw, ok := options["openai"]
	if !ok {
		return ImageOptions{}, nil
	}
	switch typed := raw.(type) {
	case ImageOptions:
		return typed, nil
	case *ImageOptions:
		if typed == nil {
			return ImageOptions{}, llm.ProviderOptionTypeError("openai", raw)
		}
		return *typed, nil
	case map[string]any:
		var decoded ImageOptions
		if err := llm.DecodeJSONProviderOptions("openai", typed, &decoded); err != nil {
			return ImageOptions{}, err
		}
		return decoded, nil
	default:
		return ImageOptions{}, llm.ProviderOptionTypeError("openai", raw)
	}
}
