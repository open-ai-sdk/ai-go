package compattest

import "testing"

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
