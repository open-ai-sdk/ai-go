package openaicompat

import (
	"testing"

	"github.com/open-ai-sdk/ai-go/llm"
)

func TestStructuredOutputNameIsOptional(t *testing.T) {
	for _, tc := range []struct{ name, want string }{{"", "structured_output"}, {"catalog_shape", "catalog_shape"}} {
		schema := map[string]any{
			"type":       "object",
			"properties": map[string]any{"ok": map[string]any{"type": "boolean"}},
		}
		format := encodeOutputSchema(&llm.OutputSchema{Type: "object", Name: tc.name, Schema: schema}, true)
		if format.JSONSchema.Name != tc.want {
			t.Errorf("name=%q got=%q", tc.name, format.JSONSchema.Name)
		}
		if _, mutated := schema["title"]; mutated {
			t.Fatal("schema naming mutated the schema")
		}
	}
}
