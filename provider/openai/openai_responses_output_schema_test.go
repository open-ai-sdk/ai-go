package openai

import (
	"encoding/json"
	"testing"

	"github.com/open-ai-sdk/ai-go/llm"
)

func TestResponsesOutputSchemaUsesFlatTextFormat(t *testing.T) {
	request, _, err := encodeRequest("gpt-test", llm.Request{Output: &llm.OutputSchema{
		Type: "object", Schema: map[string]any{"type": "object"},
	}}, false)
	if err != nil {
		t.Fatalf("encodeRequest() error = %v", err)
	}
	raw, err := json.Marshal(request.Text.Format)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	const want = `{"type":"json_schema","name":"structured_output","schema":{"type":"object"},"strict":true}`
	if string(raw) != want {
		t.Fatalf("text.format = %s, want %s", raw, want)
	}
}

func TestResponsesJSONOutputUsesJSONObjectFormat(t *testing.T) {
	for _, outputType := range []string{"json", "json_object"} {
		t.Run(outputType, func(t *testing.T) {
			text, err := encodeOutputSchema(&llm.OutputSchema{Type: outputType})
			if err != nil {
				t.Fatalf("encodeOutputSchema() error = %v", err)
			}
			format := text.Format
			if format.Type != "json_object" || format.Name != "" || format.Schema != nil {
				t.Fatalf("format = %#v", format)
			}
		})
	}
}

func TestResponsesOutputSchemaRejectsMissingSchema(t *testing.T) {
	for _, outputType := range []string{"object", "array"} {
		t.Run(outputType, func(t *testing.T) {
			_, _, err := encodeRequest("gpt-test", llm.Request{Output: &llm.OutputSchema{Type: outputType}}, false)
			if err == nil {
				t.Fatal("encodeRequest() error = nil")
			}
		})
	}
}
