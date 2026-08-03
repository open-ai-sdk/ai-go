// Package openai provides an ai-go LanguageModel implementation backed by the
// OpenAI Responses API, with support for previous-response continuation,
// reasoning settings, built-in web search, file-id inputs, and source inclusion.
package openai

import "github.com/open-ai-sdk/ai-go/llm"

// ProviderOptions holds OpenAI-specific options passed via
// GenerateTextRequest.ProviderOptions["openai"].
//
// Usage:
//
//	req := ai.GenerateTextRequest{
//	    Model: model,
//	    ProviderOptions: map[string]any{
//	        "openai": openai.ProviderOptions{
//	            PreviousResponseID: "resp_abc",
//	            ReasoningEffort:    "medium",
//	        },
//	    },
//	}
type ProviderOptions struct {
	// PreviousResponseID continues a prior Responses API response by id.
	// When set, the new request appends to that conversation thread.
	PreviousResponseID string `json:"previousResponseId"`

	// ReasoningEffort controls thinking depth for reasoning models
	// (o1, o3, o4-mini, gpt-5 series). Valid values: "low", "medium", "high".
	ReasoningEffort string `json:"reasoningEffort"`

	// ReasoningSummary controls the format of reasoning summaries.
	// Valid values: "auto", "concise", "detailed".
	ReasoningSummary string `json:"reasoningSummary"`

	// EnableWebSearch enables OpenAI's built-in web_search_preview tool.
	EnableWebSearch bool `json:"enableWebSearch"`

	// IncludeSources requests that web search sources be included in the
	// response metadata. Only meaningful when EnableWebSearch is true.
	IncludeSources bool `json:"includeSources"`

	// MaxOutputTokens overrides CallSettings.MaxTokens for the Responses API.
	// If zero, CallSettings.MaxTokens is used.
	MaxOutputTokens int `json:"maxOutputTokens"`

	// PDFDetail controls page-image processing for PDF input files.
	// Valid values are "auto", "low", and "high". Empty uses the API default.
	PDFDetail string `json:"pdfDetail"`

	// Store controls whether the response is stored server-side.
	// Defaults to true (OpenAI default). Set to false to opt out.
	Store *bool `json:"store"`

	// User is an end-user identifier for abuse monitoring.
	User string `json:"user"`

	// Metadata is arbitrary key-value metadata stored with the generation.
	Metadata map[string]string `json:"metadata"`
}

// ProviderName identifies the key used in llm.Request.ProviderOptions.
func (ProviderOptions) ProviderName() string { return "openai" }

type reasoningEffortOption struct {
	effort string
}

func (reasoningEffortOption) ProviderName() string { return "openai" }

// WithReasoningEffort constructs an option accepted by both OpenAI model APIs.
func WithReasoningEffort(effort string) llm.ProviderOption {
	return reasoningEffortOption{effort: effort}
}

// parseProviderOptions extracts typed options, with a strict JSON-map fallback.
func parseProviderOptions(opts map[string]any) (ProviderOptions, error) {
	if opts == nil {
		return ProviderOptions{}, nil
	}
	v, ok := opts["openai"]
	if !ok {
		return ProviderOptions{}, nil
	}
	switch p := v.(type) {
	case ProviderOptions:
		return p, nil
	case *ProviderOptions:
		if p == nil {
			return ProviderOptions{}, llm.ProviderOptionTypeError("openai", v)
		}
		return *p, nil
	case reasoningEffortOption:
		return ProviderOptions{ReasoningEffort: p.effort}, nil
	case map[string]any:
		var options ProviderOptions
		if err := llm.DecodeJSONProviderOptions("openai", p, &options); err != nil {
			return ProviderOptions{}, err
		}
		return options, nil
	default:
		return ProviderOptions{}, llm.ProviderOptionTypeError("openai", v)
	}
}
