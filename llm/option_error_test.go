package llm_test

import (
	"errors"
	"testing"

	"github.com/open-ai-sdk/ai-go/llm"
)

type decodedOptions struct {
	MaxTokens int `json:"maxTokens"`
}

func TestDecodeJSONProviderOptionsAcceptsJSONNumber(t *testing.T) {
	var options decodedOptions
	err := llm.DecodeJSONProviderOptions(
		"test",
		map[string]any{"maxTokens": float64(123)},
		&options,
	)
	if err != nil {
		t.Fatalf("DecodeJSONProviderOptions() error = %v", err)
	}
	if options.MaxTokens != 123 {
		t.Fatalf("MaxTokens = %d, want 123", options.MaxTokens)
	}
}

func TestDecodeJSONProviderOptionsRejectsUnknownAndWrongTypedValues(t *testing.T) {
	tests := []map[string]any{
		{"unknown": true},
		{"maxTokens": "many"},
		{"maxTokens": 1.5},
	}
	for _, input := range tests {
		var options decodedOptions
		err := llm.DecodeJSONProviderOptions("test", input, &options)
		var optionErr *llm.ProviderOptionsError
		if !errors.As(err, &optionErr) {
			t.Fatalf("input %#v error = %v, want *ProviderOptionsError", input, err)
		}
	}
}
