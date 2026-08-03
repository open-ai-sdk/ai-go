package agent

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestStructuredOutputErrorKindsAreDistinctAndWrapCause(t *testing.T) {
	decodeErr := validateStructuredOutput(json.RawMessage(`{"broken"`), &OutputSchema{Type: "object"})
	var typed *StructuredOutputError
	if !errors.As(decodeErr, &typed) || typed.Kind != StructuredOutputErrorKindJSONDecode || typed.Unwrap() == nil {
		t.Fatalf("decode error = %#v", decodeErr)
	}
	if !errors.Is(decodeErr, &StructuredOutputError{Kind: StructuredOutputErrorKindJSONDecode}) {
		t.Fatal("JSON decode kind did not match")
	}

	validationErr := validateStructuredOutput(
		json.RawMessage(`{"value":"wrong"}`),
		&OutputSchema{Type: "object", Schema: map[string]any{
			"type": "object", "properties": map[string]any{"value": map[string]any{"type": "number"}},
		}},
	)
	if !errors.As(validationErr, &typed) || typed.Kind != StructuredOutputErrorKindValidation {
		t.Fatalf("validation error = %#v", validationErr)
	}
	if errors.Is(validationErr, &StructuredOutputError{Kind: StructuredOutputErrorKindJSONDecode}) {
		t.Fatal("validation error matched JSON decode kind")
	}
}

func TestStructuredOutputEmptyResponseHasDistinctKind(t *testing.T) {
	events := runStructuredOutputEngine(t, "   ")
	for _, event := range events {
		if event.Type != StepEventError {
			continue
		}
		var typed *StructuredOutputError
		if errors.As(event.Error, &typed) && typed.Kind == StructuredOutputErrorKindEmpty {
			return
		}
	}
	t.Fatalf("events = %#v, want empty StructuredOutputError", events)
}
