package tool_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/tool"
)

type operationArgs struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type adder struct {
	Calls int
}

func (*adder) Describe() aikit.ToolDefinition {
	return aikit.ToolDefinition{
		Name:        "add",
		Description: "Add x and y together",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"x": map[string]any{"type": "integer"},
				"y": map[string]any{"type": "integer"},
			},
			"required": []string{"x", "y"},
		},
	}
}

func (a *adder) Call(_ context.Context, args operationArgs) (int, error) {
	a.Calls++
	return args.X + args.Y, nil
}

var _ tool.Typed[operationArgs, int] = (*adder)(nil)

func TestAdaptInvokesTypedToolAndCapturesDefinition(t *testing.T) {
	value := &adder{}
	adapted, err := tool.Adapt(value)
	if err != nil {
		t.Fatal(err)
	}

	definition := adapted.Describe()
	if definition.Name != "add" || definition.Description != "Add x and y together" {
		t.Fatalf("definition = %#v", definition)
	}
	definition.InputSchema["type"] = "mutated"
	if got := adapted.Describe().InputSchema["type"]; got != "object" {
		t.Fatalf("definition mutation escaped: type = %v", got)
	}

	output, err := adapted.Invoke(context.Background(), json.RawMessage(`{"x":5,"y":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "7" {
		t.Fatalf("output = %s, want 7", output)
	}
	if value.Calls != 1 {
		t.Fatalf("calls = %d, want 1", value.Calls)
	}
}

func TestAdaptClassifiesInputErrors(t *testing.T) {
	adapted, err := tool.Adapt[operationArgs, int](&adder{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapted.Invoke(context.Background(), json.RawMessage("not-json")); !errors.Is(err, tool.ErrInput) {
		t.Fatalf("invalid JSON error = %v, want ErrInput", err)
	}
}

func TestAdaptRejectsNilTypedTool(t *testing.T) {
	var value *adder
	if _, err := tool.Adapt[operationArgs, int](value); err == nil {
		t.Fatal("Adapt(nil) error = nil, want error")
	}
}
