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

func TestSanitizeMapOnlyRewritesNullableTypeUnions(t *testing.T) {
	tests := []struct {
		name  string
		type_ []any
		want  any
		null  bool
	}{
		{name: "null first", type_: []any{"null", "string"}, want: "string", null: true},
		{name: "null last", type_: []any{"string", "null"}, want: "string", null: true},
		{name: "non-null union", type_: []any{"string", "number"}, want: []any{"string", "number"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := sanitizeMap(map[string]any{"type": test.type_})
			nullable, _ := got["nullable"].(bool)
			if !equalSchemaValue(got["type"], test.want) || nullable != test.null {
				t.Fatalf("sanitizeMap() = %#v", got)
			}
		})
	}
}

func equalSchemaValue(left, right any) bool {
	leftSlice, leftOK := left.([]any)
	rightSlice, rightOK := right.([]any)
	if leftOK || rightOK {
		if !leftOK || !rightOK || len(leftSlice) != len(rightSlice) {
			return false
		}
		for i := range leftSlice {
			if leftSlice[i] != rightSlice[i] {
				return false
			}
		}
		return true
	}
	return left == right
}
