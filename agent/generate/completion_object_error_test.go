package generate

import (
	"context"
	"errors"
	"testing"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

type directObjectModel struct{ text string }

func (directObjectModel) ModelID() string { return "object" }
func (m directObjectModel) Stream(context.Context, llm.Request) (<-chan aikit.StreamEvent, error) {
	ch := make(chan aikit.StreamEvent, 1)
	if m.text != "" {
		ch <- aikit.StreamEvent{Type: aikit.StreamEventTextDelta, TextDelta: m.text}
	}
	close(ch)
	return ch, nil
}

func TestCompleteObjectClassifiesEmptyAndMalformedResponses(t *testing.T) {
	type output struct {
		Value string `json:"value"`
	}

	_, emptyErr := CompleteObject[output](context.Background(), directObjectModel{}, llm.CompletionRequest{})
	if !errors.Is(emptyErr, &agent.StructuredOutputError{Kind: agent.StructuredOutputErrorKindEmpty}) {
		t.Fatalf("empty error = %T %v", emptyErr, emptyErr)
	}

	_, decodeErr := CompleteObject[output](
		context.Background(),
		directObjectModel{text: `{"value"`},
		llm.CompletionRequest{},
	)
	if !errors.Is(decodeErr, &agent.StructuredOutputError{Kind: agent.StructuredOutputErrorKindJSONDecode}) {
		t.Fatalf("decode error = %T %v", decodeErr, decodeErr)
	}
}
