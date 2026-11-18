package tool

import (
	"reflect"
	"testing"
)

func TestStrictifyOptionsPreserveRequiredOrderAcrossRefsAndArrays(t *testing.T) {
	source := mustJSON(
		t,
		`{"type":"object","properties":{"entry":{"$ref":"#/$defs/Entry"},"list":{"type":"array","items":{"$ref":"#/$defs/Entry"}}},"required":["list","entry"],"$defs":{"Entry":{"type":"object","properties":{"z":{"type":"string"},"a":{"type":"string"}},"required":["z","a"]}}}`,
	)
	before := jsonOf(t, source)
	out, err := StrictifyWithOptions(source, StrictifyOptions{PreserveRequiredOrder: true})
	if err != nil {
		t.Fatal(err)
	}
	sameJSON(t, out["required"], `["list","entry"]`)
	properties := out["properties"].(map[string]any)
	entry := properties["entry"].(map[string]any)
	listItem := properties["list"].(map[string]any)["items"].(map[string]any)
	sameJSON(t, entry["required"], `["z","a"]`)
	sameJSON(t, listItem["required"], `["z","a"]`)
	if jsonOf(t, source) != before {
		t.Fatal("source schema mutated")
	}
	entry["required"].([]any)[0] = "changed"
	if jsonOf(t, source) != before {
		t.Fatal("output required aliases source schema")
	}
	if !reflect.DeepEqual(listItem["required"], []any{"z", "a"}) {
		t.Fatal("separate inlined references share required arrays")
	}
	defaults, err := Strictify(source)
	if err != nil {
		t.Fatal(err)
	}
	sameJSON(t, defaults["required"], `["entry","list"]`)
	sameJSON(t, defaults["properties"].(map[string]any)["entry"].(map[string]any)["required"], `["a","z"]`)
}

func TestStrictifyOptionsStillSynthesizeInvalidRequiredArrays(t *testing.T) {
	for _, required := range []string{`["z"]`, `["z","z"]`, `["z","missing"]`, `["z",1]`} {
		source := mustJSON(
			t,
			`{"type":"object","properties":{"z":{"type":"string"},"a":{"type":"string"}},"required":`+required+`}`,
		)
		out, err := StrictifyWithOptions(source, StrictifyOptions{PreserveRequiredOrder: true})
		if err != nil {
			t.Fatal(err)
		}
		sameJSON(t, out["required"], `["a","z"]`)
		if violations := StrictViolations(out); len(violations) != 0 {
			t.Fatalf("invalid strict schema: %v", violations)
		}
	}
}

func TestStrictifyOptionsAssumeAllPropertiesRequired(t *testing.T) {
	source := mustJSON(
		t,
		`{"type":"object","properties":{"a":{"type":"string"},"z":{"type":"string"},"n":{"type":["string","null"]}},"required":["a"]}`,
	)
	before := jsonOf(t, source)
	defaults, err := Strictify(source)
	if err != nil {
		t.Fatal(err)
	}
	sameJSON(t, defaults["properties"].(map[string]any)["z"], `{"anyOf":[{"type":"string"},{"type":"null"}]}`)
	out, err := StrictifyWithOptions(
		source,
		StrictifyOptions{AssumeAllPropertiesRequired: true, PreserveRequiredOrder: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	properties := out["properties"].(map[string]any)
	sameJSON(t, properties["z"], `{"type":"string"}`)
	sameJSON(t, properties["n"], `{"anyOf":[{"type":"string"},{"type":"null"}]}`)
	sameJSON(t, out["required"], `["a","n","z"]`)
	if jsonOf(t, source) != before {
		t.Fatal("source nullability mutated")
	}
	zero, err := StrictifyWithOptions(source, StrictifyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(defaults, zero) {
		t.Fatal("zero-value options changed established Strictify output")
	}
}
