package gemini

import "testing"

func TestSanitizeToolSchemas(t *testing.T) {
	tools := []map[string]any{{
		"type": "function",
		"function": map[string]any{
			"name": "test",
			"parameters": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"$ref":                 "#/defs/Foo",
				"properties": map[string]any{
					"q": map[string]any{"type": "string", "default": "hello"},
				},
			},
		},
	}}

	cleaned := sanitizeToolSchemas(tools)
	function := cleaned[0]["function"].(map[string]any)
	parameters := function["parameters"].(map[string]any)
	if _, ok := parameters["additionalProperties"]; ok {
		t.Error("additionalProperties should have been removed")
	}
	if _, ok := parameters["$ref"]; ok {
		t.Error("$ref should have been removed")
	}
	property := parameters["properties"].(map[string]any)["q"].(map[string]any)
	if _, ok := property["default"]; ok {
		t.Error("nested default should have been removed")
	}
	if property["type"] != "string" {
		t.Error("type should be preserved")
	}
}
