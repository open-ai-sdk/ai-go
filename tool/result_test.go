package tool_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/tool"
)

func TestResultInvokablePreservesTypedOrderedOutput(t *testing.T) {
	value, err := tool.New("rich", "rich result", func(context.Context, struct{}) (tool.ExecutionResult, error) {
		output, err := tool.Content(
			aikit.TextToolResultContent(`{"still":"text"}`),
			aikit.JSONToolResultContent(json.RawMessage(`{"answer":42}`)),
			aikit.ImageToolResultContent([]byte{1, 2}, "image/png"),
		)
		return tool.ExecutionResult{Output: output, Metadata: map[string]any{"private": "value"}}, err
	})
	if err != nil {
		t.Fatal(err)
	}
	set, err := tool.NewSet(value)
	if err != nil {
		t.Fatal(err)
	}
	result, err := set.InvokeResult(context.Background(), "rich", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	parts := result.Output.Parts()
	if len(parts) != 3 ||
		parts[0].Type != aikit.ToolResultContentTypeText ||
		parts[1].Type != aikit.ToolResultContentTypeJSON ||
		parts[2].Type != aikit.ToolResultContentTypeImage {
		t.Fatalf("parts = %#v", parts)
	}
	parts[2].Data[0] = 9
	if result.Output.Parts()[2].Data[0] != 1 {
		t.Fatal("output clone leaked")
	}
	if result.Metadata["private"] != "value" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestDetailsRedactsArbitraryCause(t *testing.T) {
	secret := errors.New("api-key: not-for-model")
	details := tool.Details(&tool.ExecutionError{Cause: secret})
	if details.ModelOutput.ModelText() == secret.Error() {
		t.Fatal("operator cause leaked into safe output")
	}
	if !errors.Is(&tool.ExecutionError{Cause: secret}, secret) {
		t.Fatal("cause must remain unwrap-able")
	}
}
