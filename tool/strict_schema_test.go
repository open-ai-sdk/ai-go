package tool

import "testing"

type strictChild struct {
	Value string `json:"value,omitempty"`
}

type strictParent struct {
	Name     *string       `json:"name" enum:"a,b"`
	Children []strictChild `json:"children"`
}

func TestStrictSchemaIsNullableAndStrictAtEveryObject(t *testing.T) {
	schema, err := StrictSchema[strictParent]()
	if err != nil {
		t.Fatalf("StrictSchema() error = %v", err)
	}
	properties := schema["properties"].(map[string]any)
	name := properties["name"].(map[string]any)
	if got := name["type"]; !equalJSON(got, []any{"string", "null"}) {
		t.Fatalf("name.type = %#v", got)
	}
	if got := name["enum"]; !equalJSON(got, []any{"a", "b", nil}) {
		t.Fatalf("name.enum = %#v", got)
	}
	child := properties["children"].(map[string]any)["items"].(map[string]any)
	if child["additionalProperties"] != false {
		t.Fatalf("nested child schema is not strict: %#v", child)
	}
}

func equalJSON(left, right any) bool {
	leftSlice, leftOK := left.([]any)
	rightSlice, rightOK := right.([]any)
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
