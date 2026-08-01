package anthropic

import "github.com/open-ai-sdk/ai-go/llm"

// ProviderOptions holds Anthropic-specific language-model options.
type ProviderOptions struct {
	Thinking       bool `json:"thinking"`
	ThinkingBudget int  `json:"thinkingBudget"`
}

// ProviderName identifies the key used in llm.Request.ProviderOptions.
func (ProviderOptions) ProviderName() string { return "anthropic" }

func parseProviderOptions(options map[string]any) (ProviderOptions, error) {
	if options == nil {
		return ProviderOptions{}, nil
	}
	value, found := options["anthropic"]
	if !found {
		return ProviderOptions{}, nil
	}
	switch typed := value.(type) {
	case ProviderOptions:
		return typed, nil
	case *ProviderOptions:
		if typed == nil {
			return ProviderOptions{}, llm.ProviderOptionTypeError("anthropic", value)
		}
		return *typed, nil
	case map[string]any:
		var decoded ProviderOptions
		if err := llm.DecodeJSONProviderOptions("anthropic", typed, &decoded); err != nil {
			return ProviderOptions{}, err
		}
		return decoded, nil
	default:
		return ProviderOptions{}, llm.ProviderOptionTypeError("anthropic", value)
	}
}
