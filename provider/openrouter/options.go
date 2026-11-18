package openrouter

import "github.com/open-ai-sdk/ai-go/llm"

// ProviderOptions holds OpenRouter-specific request options, passed via
// llm.Request.ProviderOptions["openrouter"], usually through Runner.With.
type ProviderOptions struct {
	// Reasoning configures reasoning tokens for models that support them.
	Reasoning *Reasoning `json:"reasoning,omitempty"`

	// Provider controls upstream provider routing.
	Provider *Routing `json:"provider,omitempty"`

	// Models lists fallback model ids tried in order when the primary model
	// fails. It is sent as the "models" request field.
	Models []string `json:"models,omitempty"`

	// Transforms lists OpenRouter prompt transforms, for example
	// "middle-out". Nil omits the field.
	Transforms []string `json:"transforms,omitempty"`

	// IncludeUsage controls usage: {include: true}, which makes OpenRouter
	// return cost and detailed token counts. Nil means true.
	IncludeUsage *bool `json:"includeUsage,omitempty"`

	// ParallelToolCalls sets parallel_tool_calls on requests that declare
	// tools. Nil omits the field.
	ParallelToolCalls *bool `json:"parallelToolCalls,omitempty"`

	// User is a stable end-user identifier forwarded to the provider.
	User string `json:"user,omitempty"`
}

// ProviderName identifies the key used in llm.Request.ProviderOptions.
func (ProviderOptions) ProviderName() string { return "openrouter" }

// Reasoning is OpenRouter's unified reasoning configuration.
type Reasoning struct {
	// Effort is "minimal", "low", "medium", or "high". Mutually exclusive with
	// MaxTokens at the provider.
	Effort string `json:"effort,omitempty"`
	// MaxTokens caps the reasoning token budget.
	MaxTokens int `json:"max_tokens,omitempty"`
	// Exclude runs reasoning but omits it from the response.
	Exclude bool `json:"exclude,omitempty"`
	// Enabled turns reasoning on with provider defaults. Nil omits the field.
	Enabled *bool `json:"enabled,omitempty"`
}

// Routing is OpenRouter's provider routing preference object.
type Routing struct {
	Order             []string `json:"order,omitempty"`
	Only              []string `json:"only,omitempty"`
	Ignore            []string `json:"ignore,omitempty"`
	Quantizations     []string `json:"quantizations,omitempty"`
	AllowFallbacks    *bool    `json:"allow_fallbacks,omitempty"`
	RequireParameters *bool    `json:"require_parameters,omitempty"`
	// DataCollection is "allow" or "deny".
	DataCollection string `json:"data_collection,omitempty"`
	// Sort is "price", "throughput", or "latency".
	Sort string `json:"sort,omitempty"`
	// ZDR restricts routing to zero-data-retention endpoints.
	ZDR *bool `json:"zdr,omitempty"`
}

func parseProviderOptions(opts map[string]any) (ProviderOptions, error) {
	value, ok := opts["openrouter"]
	if !ok {
		return ProviderOptions{}, nil
	}
	switch p := value.(type) {
	case ProviderOptions:
		return p, nil
	case *ProviderOptions:
		if p == nil {
			return ProviderOptions{}, llm.ProviderOptionTypeError("openrouter", value)
		}
		return *p, nil
	case map[string]any:
		var options ProviderOptions
		if err := llm.DecodeJSONProviderOptions("openrouter", p, &options); err != nil {
			return ProviderOptions{}, err
		}
		return options, nil
	default:
		return ProviderOptions{}, llm.ProviderOptionTypeError("openrouter", value)
	}
}
