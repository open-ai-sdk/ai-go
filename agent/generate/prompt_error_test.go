package generate

import (
	"context"
	"errors"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

func TestPromptErrorPreservesCompletionAndAPIErrorChain(t *testing.T) {
	apiErr := aikit.NewAPIError("openai", 401, nil)
	_, err := GenerateText(context.Background(), GenerateTextRequest{
		Model: apiErrModel{err: apiErr}, Messages: []Message{UserMessage("private")},
	})

	var promptErr *PromptError
	var completionErr *llm.CompletionError
	var recoveredAPIError *aikit.APIError
	if !errors.As(err, &promptErr) || promptErr.Kind != PromptErrorKindCompletion {
		t.Fatalf("error = %T %v, want completion PromptError", err, err)
	}
	if !errors.As(err, &completionErr) || completionErr.Kind != llm.CompletionErrorKindProvider {
		t.Fatalf("error = %v, want provider CompletionError in chain", err)
	}
	if !errors.As(err, &recoveredAPIError) || recoveredAPIError != apiErr {
		t.Fatal("original APIError was not preserved")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatal("authorization sentinel did not survive wrappers")
	}
}

func TestPromptErrorHistoryIncludesInstructions(t *testing.T) {
	_, err := GenerateText(context.Background(), GenerateTextRequest{
		Instructions: "system instruction",
		Messages:     []Message{UserMessage("question")},
		Tools: &ToolSet{Definitions: []ToolDefinition{{
			Name: "lookup", ContextSchema: map[string]any{"required": []any{"tenant"}},
		}}},
	})
	var promptErr *PromptError
	if !errors.As(err, &promptErr) {
		t.Fatalf("error = %T %v, want PromptError", err, err)
	}
	if len(promptErr.History) != 2 || promptErr.History[0].Role != RoleSystem ||
		promptErr.History[0].Content[0].Text != "system instruction" ||
		promptErr.History[1].Role != RoleUser {
		t.Fatalf("history = %#v", promptErr.History)
	}
}

func TestPromptErrorPartialAndHistoryAreIndependentSnapshots(t *testing.T) {
	model := &toolThenStartErrorModel{}
	initial := []Message{UserMessage("start")}
	result, err := GenerateText(context.Background(), GenerateTextRequest{
		Model: model,
		Tools: &ToolSet{
			Definitions: []ToolDefinition{{Name: "lookup"}},
			Executor:    fixedResultExecutor{},
		},
		Messages: initial,
		StopWhen: Never(),
	})

	var promptErr *PromptError
	if !errors.As(err, &promptErr) || promptErr.Partial == nil {
		t.Fatalf("error = %T %v, want PromptError with partial result", err, err)
	}
	if len(promptErr.Partial.ToolResults) != 1 || len(promptErr.History) < 2 {
		t.Fatalf("partial=%#v history=%#v", promptErr.Partial, promptErr.History)
	}
	result.ToolResults[0].Output = "changed"
	initial[0].Content[0].Text = "changed"
	if promptErr.Partial.ToolResults[0].Output == "changed" || promptErr.History[0].Content[0].Text == "changed" {
		t.Fatal("PromptError partial state aliases caller-owned state")
	}
}

func TestPromptErrorKindMatching(t *testing.T) {
	err := NewPromptError(PromptErrorKindMaxTurns, "generate", errors.New("budget"), nil, nil)
	if !errors.Is(err, &PromptError{Kind: PromptErrorKindMaxTurns}) {
		t.Fatal("max-turn kind did not match")
	}
	if errors.Is(err, &PromptError{Kind: PromptErrorKindToolExecution}) {
		t.Fatal("max-turn error matched tool-execution kind")
	}
}
