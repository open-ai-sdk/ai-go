package gemini

import (
	"errors"
	"testing"

	"github.com/open-ai-sdk/ai-go/llm"
)

func TestProviderOptionsJSONFallback(t *testing.T) {
	options, err := resolveProviderOptions(map[string]any{
		"gemini": map[string]any{
			"enableGoogleSearch": true,
			"thinkingConfig": map[string]any{
				"thinkingBudget": float64(2048),
			},
		},
	})
	if err != nil {
		t.Fatalf("resolveProviderOptions() error = %v", err)
	}
	if !options.EnableGoogleSearch {
		t.Fatal("EnableGoogleSearch = false, want true")
	}
	if options.ThinkingConfig == nil ||
		options.ThinkingConfig.ThinkingBudget == nil ||
		*options.ThinkingConfig.ThinkingBudget != 2048 {
		t.Fatalf("ThinkingConfig = %#v, want budget 2048", options.ThinkingConfig)
	}
}

func TestProviderOptionsRejectInvalidValues(t *testing.T) {
	tests := []any{
		"wrong",
		map[string]any{"enableGoogleSearch": "yes"},
		map[string]any{"unknown": true},
	}
	for _, value := range tests {
		_, err := resolveProviderOptions(map[string]any{"gemini": value})
		var optionErr *llm.ProviderOptionsError
		if !errors.As(err, &optionErr) {
			t.Fatalf("value %#v error = %v, want *ProviderOptionsError", value, err)
		}
	}
}

func TestProviderOptionsAcceptTypedPointer(t *testing.T) {
	options, err := resolveProviderOptions(map[string]any{
		"gemini": &ProviderOptions{EnableGoogleSearch: true},
	})
	if err != nil || !options.EnableGoogleSearch {
		t.Fatalf("options = %#v, error = %v", options, err)
	}
}
