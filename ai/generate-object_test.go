package ai_test

import (
	"context"
	"errors"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
)

type generateObjectAddress struct {
	City string `json:"city"`
	Zip  string `json:"zip"`
}

type generateObjectPerson struct {
	Name    string                `json:"name"`
	Age     int                   `json:"age"`
	Address generateObjectAddress `json:"address"`
}

// objectJSONModel returns a fixed JSON text on every call, regardless of the
// request — GenerateObject drives GenerateText through both the main step and
// the follow-up structured-output call, and both must see the same schema
// derived from T on Request.Output.
type objectJSONModel struct{ json string }

func (m *objectJSONModel) ModelID() string { return "object-json" }

func (m *objectJSONModel) Stream(
	_ context.Context,
	req ai.LanguageModelRequest,
) (<-chan ai.StreamEvent, error) {
	if req.Output == nil || req.Output.Type != "object" || req.Output.Schema == nil {
		return nil, errors.New("GenerateObject did not propagate a schema derived from T")
	}
	ch := make(chan ai.StreamEvent, 2)
	ch <- ai.StreamEvent{Type: ai.StreamEventTextDelta, TextDelta: m.json}
	ch <- ai.StreamEvent{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonStop}
	close(ch)
	return ch, nil
}

// TestGenerateObject_NestedStruct proves GenerateObject derives a schema from
// T (a struct-schema round trip through Request.Output), then unmarshals the
// model's JSON response into T, including a nested struct field.
func TestGenerateObject_NestedStruct(t *testing.T) {
	model := &objectJSONModel{
		json: `{"name":"Ada","age":36,"address":{"city":"London","zip":"E1"}}`,
	}

	result, err := ai.GenerateObject[generateObjectPerson](context.Background(), ai.GenerateObjectRequest{
		Model:    model,
		Messages: []ai.Message{ai.UserMessage("describe Ada")},
	})
	if err != nil {
		t.Fatalf("GenerateObject: %v", err)
	}
	if result.Object.Name != "Ada" || result.Object.Age != 36 {
		t.Errorf("Object = %+v, want Name=Ada Age=36", result.Object)
	}
	if result.Object.Address.City != "London" || result.Object.Address.Zip != "E1" {
		t.Errorf("Object.Address = %+v, want City=London Zip=E1", result.Object.Address)
	}
}

// TestGenerateObject_InvalidJSON_ReturnsError proves a non-JSON model
// response surfaces as an error instead of a zero-valued T that a caller
// could mistake for a real (empty) result.
func TestGenerateObject_InvalidJSON_ReturnsError(t *testing.T) {
	model := &objectJSONModel{json: "not json"}

	_, err := ai.GenerateObject[generateObjectPerson](context.Background(), ai.GenerateObjectRequest{
		Model:    model,
		Messages: []ai.Message{ai.UserMessage("describe Ada")},
	})
	if err == nil {
		t.Fatal("expected an error for a non-JSON model response")
	}
}

func TestGenerateObject_SchemaViolationReturnsError(t *testing.T) {
	model := &objectJSONModel{json: `{"name":"Ada","age":"thirty-six","address":{"city":"London","zip":"E1"}}`}

	_, err := ai.GenerateObject[generateObjectPerson](context.Background(), ai.GenerateObjectRequest{
		Model:    model,
		Messages: []ai.Message{ai.UserMessage("describe Ada")},
	})
	if err == nil {
		t.Fatal("expected an error when generated JSON violates the schema derived from T")
	}
}
