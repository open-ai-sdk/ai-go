package anthropic

import (
	"errors"
	"testing"

	"github.com/open-ai-sdk/ai-go/llm"
)

func TestThinkingBudgetAcceptsJSONNumber(t *testing.T) {
	config, err := extractThinkingConfig(map[string]any{
		"anthropic": map[string]any{
			"thinking":       true,
			"thinkingBudget": float64(4096),
		},
	})
	if err != nil {
		t.Fatalf("extractThinkingConfig() error = %v", err)
	}
	if config == nil || config.BudgetTokens != 4096 {
		t.Fatalf("config = %#v, want budget 4096", config)
	}
}

func TestThinkingOptionsRejectInvalidValues(t *testing.T) {
	tests := []any{
		"wrong",
		map[string]any{"thinkingBudget": "many"},
		map[string]any{"unknown": true},
	}
	for _, value := range tests {
		_, err := extractThinkingConfig(map[string]any{"anthropic": value})
		var optionErr *llm.ProviderOptionsError
		if !errors.As(err, &optionErr) {
			t.Fatalf("value %#v error = %v, want *ProviderOptionsError", value, err)
		}
	}
}

func TestThinkingOptionsAcceptTypedPointer(t *testing.T) {
	options, err := parseProviderOptions(map[string]any{
		"anthropic": &ProviderOptions{Thinking: true, ThinkingBudget: 2048},
	})
	if err != nil || !options.Thinking || options.ThinkingBudget != 2048 {
		t.Fatalf("options = %#v, error = %v", options, err)
	}
}
