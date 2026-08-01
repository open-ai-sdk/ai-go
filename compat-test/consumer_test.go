package compattest

import (
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
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
	var sawDone bool
	for event := range RunAgent(t.Context()) {
		if event.Type == aikit.StepEventDone {
			sawDone = true
		}
	}
	if !sawDone {
		t.Fatal("public agent run did not complete")
	}
}

func TestPublicFacade(t *testing.T) {
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
