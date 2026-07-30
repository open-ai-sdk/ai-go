package aisdk

import (
	"strings"
	"testing"
)

// The wire enum and the internal vocabulary are not the same strings. The client
// validates finishReason against a closed hyphenated enum and its transport throws on
// a miss, so an unmapped internal value is a browser-side crash, not a cosmetic issue.
func TestToWireFinishReason_MapsEveryInternalValue(t *testing.T) {
	cases := map[FinishReason]WireFinishReason{
		FinishReasonStop:          WireFinishStop,
		FinishReasonLength:        WireFinishLength,
		FinishReasonContentFilter: WireFinishContentFilter,
		FinishReasonToolCalls:     WireFinishToolCalls,
		FinishReasonError:         WireFinishError,
		FinishReasonUnknown:       WireFinishOther,
	}
	for in, want := range cases {
		if got := ToWireFinishReason(in); got != want {
			t.Errorf("ToWireFinishReason(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestToWireFinishReason_UnderscoreFormsNeverReachTheWire is the specific bug: the
// internal spellings use underscores, the wire enum uses hyphens.
func TestToWireFinishReason_UnderscoreFormsNeverReachTheWire(t *testing.T) {
	for _, fr := range []FinishReason{FinishReasonToolCalls, FinishReasonContentFilter} {
		got := ToWireFinishReason(fr)
		if strings.Contains(string(got), "_") {
			t.Errorf("ToWireFinishReason(%q) = %q, which contains an underscore and "+
				"would fail the client's enum", fr, got)
		}
		if _, ok := wireFinishReasons[got]; !ok {
			t.Errorf("ToWireFinishReason(%q) = %q, not in the accepted wire set", fr, got)
		}
	}
	if got := ToWireFinishReason(FinishReasonToolCalls); got != "tool-calls" {
		t.Errorf("tool_calls should map to tool-calls, got %q", got)
	}
}

// TestToWireFinishReason_UnknownProviderValueDegradesToOther — a value a provider adds
// later must not be emitted verbatim, since anything outside the enum throws.
func TestToWireFinishReason_UnknownProviderValueDegradesToOther(t *testing.T) {
	if got := ToWireFinishReason(FinishReason("some_new_provider_reason")); got != WireFinishOther {
		t.Errorf("unknown reason mapped to %q, want %q", got, WireFinishOther)
	}
}

func TestNormalizeWireFinishReason(t *testing.T) {
	cases := map[string]WireFinishReason{
		"":               "", // optional on the wire; stays absent
		"stop":           WireFinishStop,
		"tool-calls":     WireFinishToolCalls,     // already a wire value
		"tool_calls":     WireFinishToolCalls,     // internal spelling
		"content_filter": WireFinishContentFilter, // internal spelling
		"unknown":        WireFinishOther,         // no wire counterpart
		"garbage":        WireFinishOther,
	}
	for in, want := range cases {
		if got := NormalizeWireFinishReason(in); got != want {
			t.Errorf("NormalizeWireFinishReason(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestProducer_EmitsWireFinishReason closes the loop through the real producer, which
// is where the bug actually lived: it assigned string(ev.FinishReason) straight to the
// chunk field.
func TestProducer_EmitsWireFinishReason(t *testing.T) {
	events := make(chan StepEvent, 3)
	events <- StepEvent{Type: StepEventStepStart}
	events <- StepEvent{Type: StepEventStepEnd, FinishReason: FinishReasonToolCalls}
	events <- StepEvent{Type: StepEventDone}
	close(events)

	cs := NewChunkProducer("m1").Produce(events)
	var finish *Chunk
	for c := range cs.Chunks {
		if c.Type == ChunkFinish {
			cc := c
			finish = &cc
		}
	}
	if finish == nil {
		t.Fatal("no finish chunk produced")
	}
	got, _ := finish.Fields["finishReason"].(string)
	if got != "tool-calls" {
		t.Errorf("finishReason on the wire = %q, want %q — an underscore form here "+
			"fails the client's enum and throws in the browser", got, "tool-calls")
	}
}
