package gemini

import "github.com/open-ai-sdk/ai-go/llm"

// ProviderOptions holds Gemini-specific options passed via
// GenerateTextRequest.ProviderOptions["gemini"].
type ProviderOptions struct {
	// EnableGoogleSearch enables the Google Search grounding tool.
	EnableGoogleSearch bool `json:"enableGoogleSearch"`

	// GoogleSearchConfig holds optional search configuration.
	GoogleSearchConfig *GoogleSearchConfig `json:"googleSearchConfig"`

	// ThinkingConfig controls the model's thinking/reasoning behavior.
	ThinkingConfig *ThinkingConfig `json:"thinkingConfig"`

	// ResponseModalities specifies the desired output modalities, e.g. ["IMAGE"], ["TEXT", "IMAGE"].
	ResponseModalities []string `json:"responseModalities"`

	// ImageConfig holds optional configuration for image generation.
	ImageConfig *ImageConfig `json:"imageConfig"`
}

// ThinkingConfig controls how the model uses its thinking/reasoning capability.
// See: https://ai.google.dev/gemini-api/docs/gemini-3?thinking=high#thinking_level
type ThinkingConfig struct {
	// ThinkingBudget sets a token budget for thinking. Optional.
	ThinkingBudget *int `json:"thinkingBudget"`
	// IncludeThoughts controls whether thinking tokens are included in the response.
	IncludeThoughts *bool `json:"includeThoughts"`
	// ThinkingLevel sets a preset thinking level: "minimal", "low", "medium", "high".
	ThinkingLevel string `json:"thinkingLevel"`
}

// ImageConfig holds configuration for Gemini image generation.
type ImageConfig struct {
	// AspectRatio specifies the aspect ratio, e.g. "1:1", "16:9", "3:4".
	AspectRatio string `json:"aspectRatio"`
	// ImageSize specifies the output image size, e.g. "1K", "2K".
	ImageSize string `json:"imageSize"`
}

// GoogleSearchConfig contains optional configuration for Google Search grounding.
type GoogleSearchConfig struct {
	// DynamicRetrievalThreshold controls when grounding is triggered (0.0-1.0).
	// When set, MODE_DYNAMIC is used; otherwise the grounding always applies.
	DynamicRetrievalThreshold *float64 `json:"dynamicRetrievalThreshold"`

	// SearchTypes specifies which search types to use (e.g. "web", "image").
	SearchTypes []string `json:"searchTypes"`

	// TimeRangeFilter restricts search results to a specific time range.
	TimeRangeFilter *TimeRangeFilter `json:"timeRangeFilter"`
}

// TimeRangeFilter restricts Google Search grounding results to a time range.
type TimeRangeFilter struct {
	// StartTime is the start of the time range in RFC3339 format.
	StartTime string `json:"startTime"`
	// EndTime is the end of the time range in RFC3339 format.
	EndTime string `json:"endTime"`
}

// ProviderName identifies the key used in llm.Request.ProviderOptions.
func (ProviderOptions) ProviderName() string { return "gemini" }

func resolveProviderOptions(opts map[string]any) (ProviderOptions, error) {
	if opts == nil {
		return ProviderOptions{}, nil
	}
	v, ok := opts["gemini"]
	if !ok {
		return ProviderOptions{}, nil
	}
	switch typed := v.(type) {
	case ProviderOptions:
		return typed, nil
	case *ProviderOptions:
		if typed == nil {
			return ProviderOptions{}, llm.ProviderOptionTypeError("gemini", v)
		}
		return *typed, nil
	case map[string]any:
		var decoded ProviderOptions
		if err := llm.DecodeJSONProviderOptions("gemini", typed, &decoded); err != nil {
			return ProviderOptions{}, err
		}
		return decoded, nil
	default:
		return ProviderOptions{}, llm.ProviderOptionTypeError("gemini", v)
	}
}

// parseProviderOptions extracts already-validated Gemini options.
func parseProviderOptions(opts map[string]any) ProviderOptions {
	options, _ := resolveProviderOptions(opts)
	return options
}
