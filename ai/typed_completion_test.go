package ai_test

import (
	"context"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
)

func TestNewTypedCompletion(t *testing.T) {
	model := &directCompletionObjectModel{text: `{"answer":"Hanoi"}`}
	result, err := ai.NewTypedCompletion[directCompletionObject](
		model,
		"capital",
	).Instructions("Return JSON.").
		Complete(context.Background())
	if err != nil || result.Object.Answer != "Hanoi" || len(model.requests) != 1 ||
		model.requests[0].Instructions != "Return JSON." {
		t.Fatalf("result=%#v err=%v requests=%#v", result, err, model.requests)
	}
}

func TestPromptTyped(t *testing.T) {
	model := &directCompletionObjectModel{text: `{"answer":"Hanoi"}`}
	result, err := ai.PromptTyped[directCompletionObject](context.Background(), model, "capital")
	if err != nil || result.Object.Answer != "Hanoi" || len(model.requests) != 1 {
		t.Fatalf("result=%#v err=%v requests=%#v", result, err, model.requests)
	}
}
