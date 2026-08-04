package anthropic

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/open-ai-sdk/ai-go/llm"
)

func TestEncodeRequestStructuredOutput(t *testing.T) {
	model := &LanguageModel{modelID: "claude-sonnet-4-5"}
	body, _, err := model.encodeRequest(llm.Request{Output: &llm.OutputSchema{
		Type: "object", Schema: map[string]any{"type": "object"},
	}}, true)
	if err != nil {
		t.Fatalf("encodeRequest() error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	format := payload["output_config"].(map[string]any)["format"].(map[string]any)
	if format["type"] != "json_schema" || format["schema"].(map[string]any)["type"] != "object" {
		t.Fatalf("format = %#v", format)
	}
}

func TestEncodeRequestRejectsUnsupportedStructuredOutput(t *testing.T) {
	model := &LanguageModel{modelID: "claude-3-haiku"}
	_, _, err := model.encodeRequest(llm.Request{Output: &llm.OutputSchema{Type: "object"}}, true)
	var structured *llm.StructuredOutputError
	if !errors.As(err, &structured) || structured.Kind != llm.StructuredOutputErrorKindPrompt {
		t.Fatalf("error = %T %v", err, err)
	}
}

func TestSupportsStructuredOutputMatchesModelIDsExactly(t *testing.T) {
	tests := []struct {
		modelID string
		want    bool
	}{
		{modelID: "claude-sonnet-4-5", want: true},
		{modelID: "claude-sonnet-4-5-20250514", want: true},
		{modelID: "prefix-claude-sonnet-4-5", want: false},
		{modelID: "claude-sonnet-4-5-preview", want: false},
		{modelID: "claude-mythos-5", want: false},
		{modelID: "claude-3-haiku", want: false},
	}
	for _, test := range tests {
		t.Run(test.modelID, func(t *testing.T) {
			if got := (&LanguageModel{modelID: test.modelID}).supportsStructuredOutput(); got != test.want {
				t.Fatalf("supportsStructuredOutput() = %t, want %t", got, test.want)
			}
		})
	}
}
