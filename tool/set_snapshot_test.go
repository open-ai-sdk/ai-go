package tool_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/tool"
)

func dynamicTool(t *testing.T, name, output string) *tool.Tool {
	t.Helper()
	created, err := tool.NewDynamic(
		name,
		name+" description",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"value": map[string]any{"type": "string"},
			},
		},
		func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(output), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func TestSetSnapshotKeepsOrderDefinitionsAndExactInvokers(t *testing.T) {
	first := dynamicTool(t, "first", `"one"`)
	second := dynamicTool(t, "second", `"two"`)
	set, err := tool.NewSet(first, second)
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := set.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	definitions := snapshot.Definitions()
	if len(definitions) != 2 || definitions[0].Name != "first" || definitions[1].Name != "second" {
		t.Fatalf("snapshot definitions = %#v", definitions)
	}
	invoker, ok := snapshot.Invoker("second")
	if !ok || invoker != second {
		t.Fatalf("snapshot invoker = %#v, %v; want exact second tool", invoker, ok)
	}
	output, err := snapshot.Invoke(context.Background(), "second", json.RawMessage(`{}`))
	if err != nil || string(output) != `"two"` {
		t.Fatalf("snapshot invoke = %s, %v", output, err)
	}

	definitions[0].Name = "mutated"
	definitions[0].InputSchema["type"] = "array"
	again := snapshot.Definitions()
	if again[0].Name != "first" || again[0].InputSchema["type"] != "object" {
		t.Fatalf("snapshot mutation leaked: %#v", again[0])
	}
}

func TestConstructorSetIgnoresLegacyFieldMutation(t *testing.T) {
	definitions := []aikit.ToolDefinition{{
		Name:        "lookup",
		InputSchema: map[string]any{"type": "object"},
	}}
	set, err := tool.NewSetFromExecutor(definitions, &recordingExecutor{})
	if err != nil {
		t.Fatal(err)
	}

	definitions[0].Name = "source-mutated"
	set.Definitions[0].Name = "legacy-mutated"
	set.Definitions[0].InputSchema["type"] = "array"
	set.Executor = nil

	got := set.DefinitionsSnapshot()
	if len(got) != 1 || got[0].Name != "lookup" || got[0].InputSchema["type"] != "object" {
		t.Fatalf("canonical definitions = %#v", got)
	}
	if _, ok := set.Lookup("legacy-mutated"); ok {
		t.Fatal("legacy field mutation changed canonical lookup")
	}
	if _, err := set.Invoke(context.Background(), "lookup", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("legacy executor mutation changed canonical invocation: %v", err)
	}
}

func TestSetCloneOwnsDefinitionsAndPreservesExactInvoker(t *testing.T) {
	originalTool := dynamicTool(t, "clone", `"cloned"`)
	original, err := tool.NewSet(originalTool)
	if err != nil {
		t.Fatal(err)
	}
	cloned := original.Clone()
	if cloned == nil || cloned == original {
		t.Fatalf("Clone() = %p, want a new Set", cloned)
	}
	invoker, ok := cloned.Invoker("clone")
	if !ok || invoker != originalTool {
		t.Fatalf("cloned invoker = %#v, %v; want exact original tool", invoker, ok)
	}

	cloned.Definitions[0].Name = "legacy-mutated"
	if got := cloned.DefinitionsSnapshot()[0].Name; got != "clone" {
		t.Fatalf("cloned canonical definition = %q, want clone", got)
	}
	if got := original.DefinitionsSnapshot()[0].Name; got != "clone" {
		t.Fatalf("original canonical definition = %q, want clone", got)
	}
}

func TestSetSnapshotConcurrentReadersOwnReturnedDefinitions(t *testing.T) {
	set, err := tool.NewSet(dynamicTool(t, "concurrent", `{}`))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := set.Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range 50 {
				definitions := snapshot.Definitions()
				definitions[0].InputSchema["type"] = "mutated"
				if _, err := snapshot.Invoke(context.Background(), "concurrent", json.RawMessage(`{}`)); err != nil {
					t.Errorf("Invoke: %v", err)
				}
			}
		}()
	}
	wait.Wait()
	if got := snapshot.Definitions()[0].InputSchema["type"]; got != "object" {
		t.Fatalf("snapshot schema type = %v, want object", got)
	}
}

func TestNewSetFromExecutorRejectsEmptyName(t *testing.T) {
	if _, err := tool.NewSetFromExecutor(
		[]aikit.ToolDefinition{{Name: ""}},
		&recordingExecutor{},
	); err == nil {
		t.Fatal("expected empty-name registration error")
	}
}
