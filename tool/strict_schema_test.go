package tool

import (
	"testing"
	"time"
)

type strictChild struct {
	Value string `json:"value,omitempty"`
}

type strictParent struct {
	Name     *string       `json:"name"     enum:"a,b"`
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
	childProperties := child["properties"].(map[string]any)
	if got := childProperties["value"].(map[string]any)["type"]; !equalJSON(got, []any{"string", "null"}) {
		t.Fatalf("child.value.type = %#v", got)
	}
	if got := child["required"].([]string); len(got) != 1 || got[0] != "value" {
		t.Fatalf("child.required = %#v", got)
	}
}

func TestStrictSchemaRejectsNestedCustomJSONDecoding(t *testing.T) {
	type output struct {
		CreatedAt time.Time `json:"created_at"`
	}
	_, err := StrictSchema[output]()
	const want = "output field \"created_at\" has unsupported type time.Time: " +
		"custom JSON encoding is not a schema"
	if err == nil || err.Error() != want {
		t.Fatalf("StrictSchema() error = %v", err)
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
