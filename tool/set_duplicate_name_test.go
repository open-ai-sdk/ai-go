package tool_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
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

type recordingExecutor struct {
	calls []string
}

func (e *recordingExecutor) Execute(
	_ context.Context,
	name, _ string,
) (string, error) {
	e.calls = append(e.calls, name)
	return "ok", nil
}

func TestExecutorSetEnforcesRegisteredNames(t *testing.T) {
	executor := &recordingExecutor{}
	set, err := tool.NewSetFromExecutor(
		[]aikit.ToolDefinition{{Name: "allowed"}},
		executor,
	)
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
	if len(executor.calls) != 0 {
		t.Fatalf("executor calls = %v, want none", executor.calls)
	}
}

func TestRegisteredToolWithoutExecutorIsExecutionError(t *testing.T) {
	set, err := tool.NewSetFromExecutor(
		[]aikit.ToolDefinition{{Name: "known"}},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := set.Invoke(
		context.Background(),
		"known",
		json.RawMessage(`{}`),
	); !errors.Is(err, tool.ErrExecution) {
		t.Fatalf("known invocation error = %v, want ErrExecution", err)
	}
}
