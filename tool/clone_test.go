package tool_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/open-ai-sdk/ai-go/tool"
)

type namedSchemaSlice []any

func TestDynamicToolSchemaClonesTypedContainers(t *testing.T) {
	branches := []map[string]any{{"type": "string"}}
	dynamic, err := tool.NewDynamic(
		"dynamic",
		"Dynamic schema",
		map[string]any{"oneOf": branches},
		func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return nil, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	branches[0]["type"] = "number"
	first := dynamic.Describe()
	gotBranches := first.InputSchema["oneOf"].([]map[string]any)
	if gotBranches[0]["type"] != "string" {
		t.Fatalf("source mutation leaked into definition: %#v", gotBranches)
	}

	gotBranches[0]["type"] = "boolean"
	second := dynamic.Describe()
	gotBranches = second.InputSchema["oneOf"].([]map[string]any)
	if gotBranches[0]["type"] != "string" {
		t.Fatalf("Describe mutation leaked into definition: %#v", gotBranches)
	}
}

func TestDynamicToolSchemaClonesCyclicContainers(t *testing.T) {
	schema := map[string]any{"marker": "original"}
	schema["self"] = schema

	dynamic, err := tool.NewDynamic(
		"cyclic",
		"Cyclic programmatic schema",
		schema,
		func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return nil, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	schema["marker"] = "source-mutated"
	first := dynamic.Describe().InputSchema
	if first["marker"] != "original" {
		t.Fatalf("source mutation leaked into cyclic clone: %#v", first["marker"])
	}
	self := first["self"].(map[string]any)
	if self["marker"] != "original" {
		t.Fatalf("cycle does not point at cloned map: %#v", self["marker"])
	}

	first["marker"] = "describe-mutated"
	second := dynamic.Describe().InputSchema
	if second["marker"] != "original" {
		t.Fatalf("Describe mutation leaked into cyclic definition: %#v", second["marker"])
	}
}

func TestDynamicToolSchemaPreservesAliasesWithinClone(t *testing.T) {
	shared := map[string]any{"value": "original"}
	bytes := []byte("original")
	raw := json.RawMessage(`{"value":true}`)
	var nilSlice namedSchemaSlice
	schema := map[string]any{
		"null":         nil,
		"first":        shared,
		"second":       shared,
		"bytes-first":  bytes,
		"bytes-second": bytes,
		"raw":          raw,
		"nil-slice":    nilSlice,
	}

	dynamic, err := tool.NewDynamic(
		"aliases",
		"Aliased programmatic schema",
		schema,
		func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return nil, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	cloned := dynamic.Describe().InputSchema
	if cloned["null"] != nil {
		t.Fatalf("null value = %#v, want nil", cloned["null"])
	}
	cloned["first"].(map[string]any)["value"] = "changed"
	cloned["bytes-first"].([]byte)[0] = 'X'
	cloned["raw"].(json.RawMessage)[0] = '['
	if got := cloned["second"].(map[string]any)["value"]; got != "changed" {
		t.Fatalf("map alias value = %q, want changed", got)
	}
	if got := cloned["bytes-second"].([]byte)[0]; got != 'X' {
		t.Fatalf("byte alias value = %q, want X", got)
	}
	if shared["value"] != "original" || bytes[0] != 'o' {
		t.Fatal("clone mutation leaked to source schema")
	}
	if raw[0] != '{' {
		t.Fatal("raw message clone mutation leaked to source schema")
	}
	if cloned["nil-slice"].(namedSchemaSlice) != nil {
		t.Fatal("named typed nil slice became non-nil")
	}
}

func TestDynamicToolSchemaClonesPointerContainers(t *testing.T) {
	properties := map[string]any{"value": "original"}
	pointer := &properties
	dynamic, err := tool.NewDynamic(
		"pointer-schema",
		"Pointer-backed schema",
		map[string]any{"properties": pointer},
		func(context.Context, json.RawMessage) (json.RawMessage, error) { return nil, nil },
	)
	if err != nil {
		t.Fatal(err)
	}

	properties["value"] = "source-mutated"
	cloned := dynamic.Describe().InputSchema["properties"].(*map[string]any)
	if cloned == pointer || (*cloned)["value"] != "original" {
		t.Fatalf("pointer-backed schema was not independently cloned: %#v", cloned)
	}
}
