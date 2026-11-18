package tool

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func mustJSON(t *testing.T, raw string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return out
}

func mustStrictify(t *testing.T, raw string) map[string]any {
	t.Helper()
	out, err := Strictify(mustJSON(t, raw))
	if err != nil {
		t.Fatalf("Strictify() error = %v", err)
	}
	return out
}

func jsonOf(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func sameJSON(t *testing.T, got any, want string) {
	t.Helper()
	var wantValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("want fixture: %v", err)
	}
	var gotValue any
	if err := json.Unmarshal([]byte(jsonOf(t, got)), &gotValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("got %s\nwant %s", jsonOf(t, got), want)
	}
}

func TestStrictifyOptionalFieldBecomesNullableAndKeywordsAreStripped(t *testing.T) {
	out := mustStrictify(t, `{
		"type":"object",
		"properties":{
			"url":{"type":"string","minLength":1},
			"page":{"type":"integer","minimum":1},
			"kind":{"type":"string","const":"wait"}
		},
		"required":["url","kind"]
	}`)
	props := out["properties"].(map[string]any)
	sameJSON(t, props["url"], `{"type":"string"}`)
	sameJSON(t, props["page"], `{"anyOf":[{"type":"integer"},{"type":"null"}]}`)
	sameJSON(t, props["kind"], `{"type":"string","enum":["wait"]}`)
	sameJSON(t, out["required"], `["kind","page","url"]`)
	if out["additionalProperties"] != false {
		t.Fatalf("additionalProperties = %v", out["additionalProperties"])
	}
}

func TestStrictifyInlinesRefsAndDropsDefs(t *testing.T) {
	out := mustStrictify(t, `{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"title":"Outer",
		"type":"object",
		"properties":{
			"inner":{"$ref":"#/$defs/Inner"},
			"maybe":{"anyOf":[{"$ref":"#/$defs/Inner"},{"type":"null"}],"default":null}
		},
		"required":["inner"],
		"$defs":{"Inner":{
			"type":"object","title":"Inner",
			"properties":{"a":{"type":"string"},"b":{"type":"integer"}},
			"required":["a"]
		}}
	}`)
	rendered := jsonOf(t, out)
	for _, banned := range []string{"$ref", "$defs", "$schema", "title", "default"} {
		if strings.Contains(rendered, banned) {
			t.Fatalf("output still contains %s: %s", banned, rendered)
		}
	}
	inner := `{"type":"object","additionalProperties":false,"required":["a","b"],` +
		`"properties":{"a":{"type":"string"},"b":{"anyOf":[{"type":"integer"},{"type":"null"}]}}}`
	sameJSON(t, out["properties"].(map[string]any)["inner"], inner)
	sameJSON(t, out["properties"].(map[string]any)["maybe"],
		`{"anyOf":[`+inner+`,{"type":"null"}]}`)
	sameJSON(t, out["required"], `["inner","maybe"]`)
}

func TestStrictifyRewritesArrayTypeIntoAnyOf(t *testing.T) {
	tests := []struct {
		name   string
		schema string
		path   string
		want   string
	}{
		{
			name:   "nullable string drops default",
			schema: `{"type":"object","properties":{"v":{"type":["string","null"],"default":null}},"required":["v"]}`,
			path:   "v",
			want:   `{"anyOf":[{"type":"string"},{"type":"null"}]}`,
		},
		{
			name:   "nullable list keeps items in array branch",
			schema: `{"type":"object","properties":{"v":{"type":["array","null"],"items":{"type":"string"}}},"required":["v"]}`,
			path:   "v",
			want:   `{"anyOf":[{"type":"array","items":{"type":"string"}},{"type":"null"}]}`,
		},
		{
			name: "nullable object keeps properties in object branch",
			schema: `{"type":"object","properties":{"v":{"type":["object","null"],"description":"kept",` +
				`"properties":{"a":{"type":"string"}}}},"required":["v"]}`,
			path: "v",
			want: `{"description":"kept","anyOf":[{"type":"object","additionalProperties":false,` +
				`"required":["a"],"properties":{"a":{"anyOf":[{"type":"string"},{"type":"null"}]}}},{"type":"null"}]}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out := mustStrictify(t, test.schema)
			sameJSON(t, out["properties"].(map[string]any)[test.path], test.want)
		})
	}
}

func TestStrictifyEveryObjectIsStrict(t *testing.T) {
	out := mustStrictify(t, `{
		"type":"object",
		"properties":{
			"tags":{"type":"array","items":{"type":"string"}},
			"rows":{"type":"array","items":{"type":"object","properties":{"x":{"type":"string"}}}},
			"nested":{"type":"object","properties":{"y":{"type":"string"}}}
		},
		"required":["tags"]
	}`)
	sameJSON(t, out["required"], `["nested","rows","tags"]`)
	props := out["properties"].(map[string]any)
	row := props["rows"].(map[string]any)["anyOf"].([]any)[0].(map[string]any)["items"]
	sameJSON(t, row, `{"type":"object","additionalProperties":false,"required":["x"],`+
		`"properties":{"x":{"anyOf":[{"type":"string"},{"type":"null"}]}}}`)
	nested := props["nested"].(map[string]any)["anyOf"].([]any)[0]
	sameJSON(t, nested, `{"type":"object","additionalProperties":false,"required":["y"],`+
		`"properties":{"y":{"anyOf":[{"type":"string"},{"type":"null"}]}}}`)
}

func TestStrictifyStripsEveryForbiddenKeyword(t *testing.T) {
	out := mustStrictify(t, `{
		"type":"object",
		"properties":{
			"count":{"type":"integer","format":"uint32","minimum":0},
			"ratio":{"type":"number","format":"double","maximum":1.0,"multipleOf":0.5},
			"name":{"type":"string","minLength":1,"maxLength":9,"pattern":"^a"},
			"list":{"type":"array","items":{"type":"string"},"minItems":1,"maxItems":3,"uniqueItems":true},
			"kind":{"type":"string","const":"call"}
		},
		"required":["count","ratio","name","list","kind"]
	}`)
	rendered := jsonOf(t, out)
	for _, banned := range []string{
		"format", "minimum", "maximum", "minLength", "maxLength", "pattern",
		"minItems", "maxItems", "multipleOf", "uniqueItems", "default", "const",
	} {
		if strings.Contains(rendered, `"`+banned+`"`) {
			t.Fatalf("output contains %q: %s", banned, rendered)
		}
	}
	sameJSON(t, out["properties"].(map[string]any)["kind"], `{"type":"string","enum":["call"]}`)
	sameJSON(t, out["properties"].(map[string]any)["count"], `{"type":"integer"}`)
}

func TestStrictifyOneOf(t *testing.T) {
	t.Run("enum and const branches collapse into one enum", func(t *testing.T) {
		out := mustStrictify(t, `{
			"type":"object",
			"properties":{"risk":{
				"default":"review",
				"oneOf":[
					{"type":"string","enum":["low","medium","high"]},
					{"type":"string","const":"review","description":"needs review"}
				]}},
			"required":["risk"]
		}`)
		sameJSON(t, out["properties"].(map[string]any)["risk"],
			`{"type":"string","enum":["low","medium","high","review"],"description":"needs review"}`)
	})
	t.Run("type union becomes anyOf", func(t *testing.T) {
		out := mustStrictify(t, `{
			"type":"object",
			"properties":{"mixed":{"oneOf":[{"type":"string"},{"type":"integer"}]}},
			"required":["mixed"]
		}`)
		sameJSON(t, out["properties"].(map[string]any)["mixed"],
			`{"anyOf":[{"type":"string"},{"type":"integer"}]}`)
	})
}

func TestStrictifyReplacesBooleanSchemaNodes(t *testing.T) {
	out := mustStrictify(t, `{
		"type":"object",
		"properties":{"tool":{"type":"string"},"input":true,"rows":{"type":"array","items":true}},
		"required":["tool","input","rows"]
	}`)
	closed := `{"type":"object","properties":{},"additionalProperties":false,"required":[]}`
	props := out["properties"].(map[string]any)
	sameJSON(t, props["input"], closed)
	sameJSON(t, props["rows"], `{"type":"array","items":`+closed+`}`)
}

func TestStrictifyDoesNotMutateInput(t *testing.T) {
	input := mustJSON(t, `{
		"type":"object",
		"properties":{"a":{"$ref":"#/$defs/A"},"b":{"type":["string","null"]}},
		"required":["a"],
		"$defs":{"A":{"type":"object","properties":{"x":{"type":"string","minLength":1}}}}
	}`)
	before := jsonOf(t, input)
	if _, err := Strictify(input); err != nil {
		t.Fatal(err)
	}
	if after := jsonOf(t, input); after != before {
		t.Fatalf("input mutated:\nbefore %s\nafter  %s", before, after)
	}
}

func TestStrictifyAcceptsStrictSchemaOutput(t *testing.T) {
	schema, err := StrictSchema[strictParent]()
	if err != nil {
		t.Fatal(err)
	}
	out, err := Strictify(schema)
	if err != nil {
		t.Fatalf("Strictify(StrictSchema) error = %v", err)
	}
	if violations := StrictViolations(out); len(violations) != 0 {
		t.Fatalf("violations = %v", violations)
	}
}

func TestStrictifyReportsUnfixableViolations(t *testing.T) {
	tests := []struct {
		name   string
		schema string
		want   string
	}{
		{
			"root not object", `{"anyOf":[{"type":"object","properties":{},"additionalProperties":false}]}`,
			"the root must be an object",
		},
		{
			"object without properties", `{"type":"object","properties":{"o":{"type":"object"}},"required":["o"]}`,
			"an object without properties",
		},
		{
			"unresolvable ref", `{"type":"object","properties":{"r":{"$ref":"#/$defs/Missing"}},"required":["r"]}`,
			"`$ref` is not supported",
		},
		{
			"allOf", `{"type":"object","properties":{"r":{"allOf":[{"type":"string"}]}},"required":["r"]}`,
			"`allOf` is not supported",
		},
		{
			"empty enum", `{"type":"object","properties":{"r":{"type":"string","enum":[]}},"required":["r"]}`,
			"an empty enum",
		},
		{
			"untyped node", `{"type":"object","properties":{"r":{"description":"x"}},"required":["r"]}`,
			"a node without a type",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, violations, err := StrictifyWithReport(mustJSON(t, test.schema))
			if err != nil {
				t.Fatalf("StrictifyWithReport() error = %v", err)
			}
			var found bool
			for _, violation := range violations {
				found = found || strings.Contains(violation.Reason, test.want)
			}
			if !found {
				t.Fatalf("violations %v lack %q", violations, test.want)
			}
			_, err = Strictify(mustJSON(t, test.schema))
			var strictErr *StrictifyError
			if !errors.As(err, &strictErr) || len(strictErr.Violations) == 0 {
				t.Fatalf("Strictify() error = %v, want *StrictifyError", err)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error %q lacks %q", err, test.want)
			}
		})
	}
}

func TestStrictifyRejectsUnprocessableInput(t *testing.T) {
	if _, err := Strictify(nil); err == nil {
		t.Fatal("nil schema: want error")
	}
	if _, err := Strictify(map[string]any{"f": func() {}}); err == nil {
		t.Fatal("non-JSON schema: want error")
	}
	recursive := mustJSON(t, `{
		"type":"object",
		"properties":{"node":{"$ref":"#/$defs/Node"}},
		"required":["node"],
		"$defs":{"Node":{"type":"object","properties":{"next":{"$ref":"#/$defs/Node"}},"required":["next"]}}
	}`)
	if _, err := Strictify(recursive); err == nil || !strings.Contains(err.Error(), "recursive") {
		t.Fatalf("recursive $ref error = %v", err)
	}
}

func TestStrictViolationsNamesRefusedShapes(t *testing.T) {
	violations := StrictViolations(mustJSON(t, `{
		"type":"object",
		"properties":{"a":{"type":["string","null"]},"b":{"oneOf":[{"type":"string"}]}},
		"required":["a"]
	}`))
	var joined []string
	for _, violation := range violations {
		joined = append(joined, violation.String())
	}
	found := strings.Join(joined, "\n")
	for _, want := range []string{
		"$: additionalProperties must be false",
		"$: `b` is not required",
		"$.a: an array `type`",
		"$.b: `oneOf` is not supported",
	} {
		if !strings.Contains(found, want) {
			t.Fatalf("missing %q in:\n%s", want, found)
		}
	}
}

func TestStrictViolationsLimits(t *testing.T) {
	nested := map[string]any{"type": "string"}
	for range strictMaxDepth + 1 {
		nested = map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"c"},
			"properties":           map[string]any{"c": nested},
		}
	}
	violations := StrictViolations(nested)
	if len(violations) != 1 || !strings.Contains(violations[0].Reason, "objects deep") {
		t.Fatalf("depth violations = %v", violations)
	}

	values := make([]any, strictMaxEnumValues+1)
	for i := range values {
		values[i] = i
	}
	enumSchema := map[string]any{
		"type": "object", "additionalProperties": false, "required": []any{"e"},
		"properties": map[string]any{"e": map[string]any{"type": "integer", "enum": values}},
	}
	violations = StrictViolations(enumSchema)
	if len(violations) != 1 || !strings.Contains(violations[0].Reason, "enum values") {
		t.Fatalf("enum violations = %v", violations)
	}

	props := make(map[string]any, strictMaxProperties+1)
	required := make([]any, 0, strictMaxProperties+1)
	for i := range strictMaxProperties + 1 {
		name := "p" + strconv.Itoa(i)
		props[name] = map[string]any{"type": "string"}
		required = append(required, name)
	}
	violations = StrictViolations(map[string]any{
		"type": "object", "additionalProperties": false, "required": required, "properties": props,
	})
	if len(violations) != 1 || !strings.Contains(violations[0].Reason, "properties (at most") {
		t.Fatalf("property violations = %v", violations)
	}
}

func TestCoerceEmptyObjectsToNull(t *testing.T) {
	schema := mustStrictify(t, `{
		"type":"object",
		"properties":{
			"event":{"anyOf":[{"type":"string"},{"type":"null"}]},
			"grade":{"anyOf":[{"type":"string","enum":["high","low"]},{"type":"null"}]},
			"label":{"type":"string"},
			"nested":{"type":"object","properties":{"note":{"anyOf":[{"type":"string"},{"type":"null"}]}},"required":["note"]},
			"tags":{"type":"array","items":{"type":"object","properties":{"name":{"anyOf":[{"type":"string"},{"type":"null"}]}},"required":["name"]}}
		},
		"required":["event","grade","label","nested","tags"]
	}`)
	var value any
	if err := json.Unmarshal([]byte(`{
		"event":{},"grade":{},"label":"keep",
		"nested":{"note":{}},
		"tags":[{"name":"a"},{"name":{}}]
	}`), &value); err != nil {
		t.Fatal(err)
	}
	got, count := CoerceEmptyObjectsToNull(value, schema)
	if count != 4 {
		t.Fatalf("count = %d, want 4", count)
	}
	sameJSON(
		t,
		got,
		`{"event":null,"grade":null,"label":"keep","nested":{"note":null},"tags":[{"name":"a"},{"name":null}]}`,
	)
}

func TestCoerceEmptyObjectsToNullLeavesNonNullableSlots(t *testing.T) {
	schema := mustJSON(t, `{"type":"object","properties":{"strict":{"type":"object"}}}`)
	got, count := CoerceEmptyObjectsToNull(mustJSON(t, `{"strict":{}}`), schema)
	if count != 0 {
		t.Fatalf("count = %d", count)
	}
	sameJSON(t, got, `{"strict":{}}`)

	root, count := CoerceEmptyObjectsToNull(
		map[string]any{},
		mustJSON(t, `{"anyOf":[{"type":"string"},{"type":"null"}]}`),
	)
	if root != nil || count != 1 {
		t.Fatalf("root = %v count = %d", root, count)
	}
	if got, count := CoerceEmptyObjectsToNull("x", nil); got != "x" || count != 0 {
		t.Fatalf("nil schema: %v %d", got, count)
	}
}

func TestCoerceEmptyObjectsToEmptyString(t *testing.T) {
	schema := mustJSON(t, `{
		"type":"object",
		"properties":{
			"note":{"type":"string"},
			"tags":{"type":"array","items":{"type":"string"}},
			"maybe":{"anyOf":[{"type":"string"},{"type":"null"}]},
			"obj":{"type":"object","properties":{}}
		}
	}`)
	got, count := CoerceEmptyObjectsToEmptyString(
		mustJSON(t, `{"note":{},"tags":[{}],"maybe":{},"obj":{}}`), schema,
	)
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
	sameJSON(t, got, `{"note":"","tags":[""],"maybe":{},"obj":{}}`)
}
