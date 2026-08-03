package compattest

import (
	"testing"
)

// TestConsumeFakeStream exercises the hand-built StepEvent stream through
// ai.NewStreamResult from outside the module. Its compilation is the real
// assertion; the runtime check confirms the aggregated result flows back.
func TestConsumeFakeStream(t *testing.T) {
	text, err := ConsumeFakeStream()
	if err != nil {
		t.Fatalf("ConsumeFakeStream: %v", err)
	}
	if text != "hi" {
		t.Fatalf("expected accumulated text %q, got %q", "hi", text)
	}
}

func TestRunAgent(t *testing.T) {
	result, err := RunAgent(t.Context())
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if result.Text != "hello" {
		t.Fatalf("RunAgent text = %q, want hello", result.Text)
	}
}

func TestPublicFacade(t *testing.T) {
	messageID, typedRaw, err := NativeCompletion(t.Context())
	if err != nil || messageID != "msg_external" || !typedRaw {
		t.Fatalf("NativeCompletion = %q, %t, %v", messageID, typedRaw, err)
	}

	text, err := GenerateText(t.Context())
	if err != nil || text != "hello" {
		t.Fatalf("GenerateText = %q, %v", text, err)
	}

	streamed, err := StreamText(t.Context())
	if err != nil || streamed != "hello" {
		t.Fatalf("StreamText = %q, %v", streamed, err)
	}

	object, err := GenerateObject(t.Context())
	if err != nil || object != "ok" {
		t.Fatalf("GenerateObject = %q, %v", object, err)
	}

	embedding, err := Embed(t.Context())
	if err != nil || len(embedding) != 3 {
		t.Fatalf("Embed = %v, %v", embedding, err)
	}
}
