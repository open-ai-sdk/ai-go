package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func runStructuredOutputEngine(t *testing.T, modelResponse string) []StepEvent {
	t.Helper()

	model := &mockModel{calls: [][]StreamEvent{
		// The native schema constrains the normal final turn; no second call is made.
		{
			{Type: StreamEventTextDelta, TextDelta: modelResponse},
			{Type: StreamEventFinish, FinishReason: FinishReasonStop},
		},
	}}

	ch := driveStream(context.Background(), runConfig{
		Model: model,
		Request: Request{
			Output: &OutputSchema{
				Type:   "object",
				Schema: map[string]any{"type": "object"},
			},
		},
		MaxSteps: 5,
	})

	var events []StepEvent
	for ev := range ch {
		events = append(events, ev)
	}
	return events
}

func TestStructuredOutput_EnforcesStandardSchemaConstraints(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		schema map[string]any
	}{
		{
			name: "pattern",
			raw:  `{"code":"lower"}`,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{"code": map[string]any{
					"type": "string", "pattern": "^[A-Z]+$",
				}},
			},
		},
		{name: "minimum", raw: `3`, schema: map[string]any{"type": "number", "minimum": 5}},
		{name: "minItems", raw: `[]`, schema: map[string]any{"type": "array", "minItems": 1}},
		{name: "format", raw: `"not-a-date"`, schema: map[string]any{"type": "string", "format": "date-time"}},
		{
			name: "oneOf",
			raw:  `true`,
			schema: map[string]any{"oneOf": []any{
				map[string]any{"type": "string"}, map[string]any{"type": "number"},
			}},
		},
		{name: "type array", raw: `true`, schema: map[string]any{"type": []any{"string", "null"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateStructuredOutput(json.RawMessage(test.raw), &OutputSchema{
				Type: "object", Schema: test.schema,
			})
			if err == nil {
				t.Fatal("expected schema validation error")
			}
			var structuredErr *StructuredOutputError
			if !errors.As(err, &structuredErr) {
				t.Fatalf("error = %T %v, want StructuredOutputError", err, err)
			}
		})
	}
}

func TestStructuredOutput_ExternalSchemaReferenceFailsClosed(t *testing.T) {
	err := validateStructuredOutput(json.RawMessage(`{"ok":true}`), &OutputSchema{
		Type: "object",
		Schema: map[string]any{
			"$ref": "https://example.invalid/untrusted-schema.json",
		},
	})
	if err == nil {
		t.Fatal("expected an unresolved external schema reference to fail closed")
	}
	var structuredErr *StructuredOutputError
	if !errors.As(err, &structuredErr) || structuredErr.Path != "$schema" {
		t.Fatalf("error = %T %v, want $schema StructuredOutputError", err, err)
	}
}

func findStructuredOutput(events []StepEvent) (StepEvent, bool) {
	for _, ev := range events {
		if ev.Type == StepEventStructuredOutput {
			return ev, true
		}
	}
	return StepEvent{}, false
}

func TestStructuredOutput_ValidJSON(t *testing.T) {
	events := runStructuredOutputEngine(t, `{"score":42,"label":"good"}`)
	ev, ok := findStructuredOutput(events)
	if !ok {
		t.Fatal("expected StepEventStructuredOutput")
	}
	if string(ev.StructuredOutput) != `{"score":42,"label":"good"}` {
		t.Errorf("unexpected structured output: %s", ev.StructuredOutput)
	}
}

func TestStructuredOutput_FencedJSON(t *testing.T) {
	fenced := "```json\n{\"score\":7}\n```"
	events := runStructuredOutputEngine(t, fenced)
	ev, ok := findStructuredOutput(events)
	if !ok {
		t.Fatal("expected StepEventStructuredOutput for fenced JSON")
	}
	if string(ev.StructuredOutput) != `{"score":7}` {
		t.Errorf("unexpected structured output: %s", ev.StructuredOutput)
	}
}

func TestStructuredOutput_InvalidJSON(t *testing.T) {
	events := runStructuredOutputEngine(t, "not valid json at all")
	_, ok := findStructuredOutput(events)
	if ok {
		t.Error("expected no StepEventStructuredOutput for invalid JSON")
	}
	assertStructuredOutputFailure(t, events)
}

func TestStructuredOutput_SchemaViolationTerminatesWithoutDone(t *testing.T) {
	model := &mockModel{calls: [][]StreamEvent{
		{textEvt(`{"age":"not-an-integer"}`), finishEvt(FinishReasonStop)},
	}}
	events := collectRunEvents(runConfig{
		Model: model,
		Request: Request{Output: &OutputSchema{
			Type: "object",
			Schema: map[string]any{
				"type":     "object",
				"required": []string{"name", "age"},
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
					"age":  map[string]any{"type": "integer"},
				},
			},
		}},
	})
	assertStructuredOutputFailure(t, events)
}

func assertStructuredOutputFailure(t *testing.T, events []StepEvent) {
	t.Helper()
	var schemaErr *StructuredOutputError
	var sawDone bool
	for _, event := range events {
		if event.Type == StepEventError {
			errors.As(event.Error, &schemaErr)
		}
		sawDone = sawDone || event.Type == StepEventDone
	}
	if schemaErr == nil {
		t.Fatalf("events = %#v, want StructuredOutputError", events)
	}
	if sawDone {
		t.Fatal("structured-output validation failure must not be followed by Done")
	}
}

func TestStructuredOutput_ProviderErrorTerminatesWithoutDone(t *testing.T) {
	providerErr := errors.New("structured provider failed")
	model := &mockModel{calls: [][]StreamEvent{
		{{Type: StreamEventError, Error: providerErr}},
	}}
	events := collectRunEvents(runConfig{
		Model: model,
		Request: Request{Output: &OutputSchema{
			Type: "object",
		}},
	})

	var sawOriginalError, sawDone bool
	for _, event := range events {
		if event.Type == StepEventError && errors.Is(event.Error, providerErr) {
			sawOriginalError = true
		}
		sawDone = sawDone || event.Type == StepEventDone
	}
	if !sawOriginalError {
		t.Fatalf("events = %#v, want structured provider error", events)
	}
	if sawDone {
		t.Fatal("structured-output failure must not be followed by Done")
	}
}

func TestStructuredOutput_PreservesEffectiveProviderOptions(t *testing.T) {
	model := &recordingModel{mockModel: mockModel{calls: [][]StreamEvent{
		{textEvt(`{"ok":true}`), finishEvt(FinishReasonStop)},
	}}}
	events := collectRunEvents(runConfig{
		Model: model,
		Request: Request{
			Output:          &OutputSchema{Type: "object"},
			ProviderOptions: map[string]any{"base": "kept"},
		},
		PrepareStep: func(PrepareStepContext) *PrepareStepResult {
			return &PrepareStepResult{ProviderOptions: map[string]any{"override": "applied"}}
		},
	})
	for _, event := range events {
		if event.Type == StepEventError {
			t.Fatalf("unexpected error: %v", event.Error)
		}
	}
	if len(model.requests) != 1 {
		t.Fatalf("model requests = %d, want 1", len(model.requests))
	}
	options := model.requests[0].ProviderOptions
	if options["base"] != "kept" || options["override"] != "applied" {
		t.Fatalf("structured provider options = %#v", options)
	}
}
