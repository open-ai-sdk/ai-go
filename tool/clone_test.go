package tool_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/open-ai-sdk/ai-go/tool"
)

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
