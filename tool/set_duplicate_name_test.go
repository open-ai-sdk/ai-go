package tool_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/tool"
)

func TestNewSetRejectsDuplicateNames(t *testing.T) {
	first, err := tool.New(
		"duplicate",
		"First",
		func(context.Context, struct{}) (string, error) { return "first", nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := tool.New(
		"duplicate",
		"Second",
		func(context.Context, struct{}) (string, error) { return "second", nil },
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.NewSet(first, second)
	if err == nil {
		t.Fatal("expected duplicate registration error")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error = %q", err)
	}
}

func TestNewSetRejectsTypedNilTool(t *testing.T) {
	var candidate *tool.Tool
	if _, err := tool.NewSet(candidate); err == nil {
		t.Fatal("expected typed nil registration error")
	}
}

func TestSetEnforcesRegisteredNames(t *testing.T) {
	allowed, err := tool.NewDynamic(
		"allowed",
		"Allowed tool",
		map[string]any{"type": "object"},
		func(context.Context, json.RawMessage) (json.RawMessage, error) {
			t.Fatal("blocked invocation reached registered tool")
			return nil, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	set, err := tool.NewSet(allowed)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := set.Invoke(
		context.Background(),
		"blocked",
		json.RawMessage(`{}`),
	); !errors.Is(err, tool.ErrNoSuchTool) {
		t.Fatalf("blocked invocation error = %v, want ErrNoSuchTool", err)
	}
}
