package ai_test

import (
	"context"
	"errors"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
)

type directCompletionObject struct {
	Answer string `json:"answer"`
}

type directCompletionObjectModel struct {
	requests []ai.CompletionRequest
	text     string
}

func (*directCompletionObjectModel) ModelID() string { return "direct-object" }

func (m *directCompletionObjectModel) Stream(
	_ context.Context,
	request ai.CompletionRequest,
) (<-chan ai.StreamEvent, error) {
	m.requests = append(m.requests, request)
	if request.Output == nil || request.Output.Type != "object" || request.Output.Schema == nil {
		return nil, errors.New("CompleteObject did not provide an object schema")
	}
	ch := make(chan ai.StreamEvent, 2)
	ch <- ai.StreamEvent{Type: ai.StreamEventTextDelta, TextDelta: m.text}
	ch <- ai.StreamEvent{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonStop}
	close(ch)
	return ch, nil
}

func TestCompleteObjectMakesOneDirectCall(t *testing.T) {
	model := &directCompletionObjectModel{text: `{"answer":"Hanoi"}`}
	request := ai.NewCompletion(model, "What is Vietnam's capital?").
		Instructions("Answer as JSON.").
		Temperature(0.2).
		Build()

	result, err := ai.CompleteObject[directCompletionObject](context.Background(), model, request)
	if err != nil {
		t.Fatalf("CompleteObject: %v", err)
	}
	if result.Object.Answer != "Hanoi" || result.Response == nil || result.Response.Text != model.text {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(model.requests) != 1 || model.requests[0].Instructions != "Answer as JSON." ||
		model.requests[0].Settings.Temperature == nil ||
		*model.requests[0].Settings.Temperature != 0.2 ||
		len(model.requests[0].Messages) != 1 {
		t.Fatalf("unexpected direct requests: %#v", model.requests)
	}
}

func TestCompleteObjectReturnsResponseWithDecodeError(t *testing.T) {
	model := &directCompletionObjectModel{text: "not json"}
	result, err := ai.CompleteObject[directCompletionObject](
		context.Background(),
		model,
		ai.NewCompletion(model, "answer").Build(),
	)
	if err == nil || result.Response == nil || result.Response.Text != "not json" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
