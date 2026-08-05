package agent

import "testing"

func makeToolCallDelta(idx int, id, name, argsDelta string) StreamEvent {
	return StreamEvent{
		Type:              StreamEventToolCallDelta,
		ToolCallIndex:     idx,
		ToolCallID:        id,
		ToolCallName:      name,
		ToolCallArgsDelta: argsDelta,
	}
}

// Assembly rules are covered by aikit's ToolCallFold tests. These cover the
// adaptation the run engine depends on: index ordering, the new-index signal
// that drives tool-call-start emission, and the field mapping onto
// toolCallState.
func TestAccumulatorReportsNewIndexOnce(t *testing.T) {
	acc := newToolCallAccumulator()
	if !acc.add(makeToolCallDelta(0, "tc1", "search", `{"q`)) {
		t.Fatal("add() = false for a previously unseen index, want true")
	}
	if acc.add(makeToolCallDelta(0, "tc1", "search", `":"hello"}`)) {
		t.Fatal("add() = true for a continuation delta, want false")
	}
	if !acc.hasToolCalls() {
		t.Fatal("hasToolCalls() = false after a tool-call delta, want true")
	}
}

func TestAccumulatorMapsDraftsOntoToolCallState(t *testing.T) {
	acc := newToolCallAccumulator()
	event := makeToolCallDelta(0, "tc1", "search", `{"q":"hello"}`)
	event.ThoughtSignature = "sig"
	acc.add(event)

	calls := acc.completed()
	if len(calls) != 1 {
		t.Fatalf("completed() len = %d, want 1", len(calls))
	}
	want := toolCallState{id: "tc1", name: "search", args: `{"q":"hello"}`, thoughtSignature: "sig"}
	if calls[0] != want {
		t.Errorf("completed()[0] = %+v, want %+v", calls[0], want)
	}
}

func TestAccumulatorOrdersByProviderIndex(t *testing.T) {
	acc := newToolCallAccumulator()
	acc.add(makeToolCallDelta(0, "tc1", "search", `{"q`))
	acc.add(makeToolCallDelta(1, "tc2", "fetch", `{"url`))
	acc.add(makeToolCallDelta(0, "tc1", "search", `":"a"}`))
	acc.add(makeToolCallDelta(1, "tc2", "fetch", `":"b"}`))

	calls := acc.completed()
	if len(calls) != 2 {
		t.Fatalf("completed() len = %d, want 2", len(calls))
	}
	if calls[0].args != `{"q":"a"}` {
		t.Errorf("call 0 args = %q, want %q", calls[0].args, `{"q":"a"}`)
	}
	if calls[1].args != `{"url":"b"}` {
		t.Errorf("call 1 args = %q, want %q", calls[1].args, `{"url":"b"}`)
	}
}

// Anthropic's OpenAI-compatible API sends tool calls starting at index 1.
func TestAccumulatorHandlesNonZeroBasedIndex(t *testing.T) {
	acc := newToolCallAccumulator()
	acc.add(makeToolCallDelta(1, "tc1", "bash", `{"command":"ls"}`))

	calls := acc.completed()
	if len(calls) != 1 {
		t.Fatalf("completed() len = %d, want 1", len(calls))
	}
	if calls[0].name != "bash" {
		t.Errorf("name = %q, want bash", calls[0].name)
	}
	if calls[0].args != `{"command":"ls"}` {
		t.Errorf("args = %q, want %q", calls[0].args, `{"command":"ls"}`)
	}
}

func TestAccumulatorEmptyHasNoToolCalls(t *testing.T) {
	acc := newToolCallAccumulator()
	if acc.hasToolCalls() {
		t.Error("hasToolCalls() = true before any delta, want false")
	}
	if acc.completed() != nil {
		t.Error("completed() is non-nil before any delta, want nil")
	}
}

// The tool name may arrive after the first chunk on OpenAI-compatible
// providers; the run engine needs it to look up the tool definition.
func TestAccumulatorAdoptsLateToolName(t *testing.T) {
	acc := newToolCallAccumulator()
	acc.add(makeToolCallDelta(0, "tc1", "", `{"a`))
	acc.add(makeToolCallDelta(0, "tc1", "search", `":1}`))

	calls := acc.completed()
	if len(calls) != 1 {
		t.Fatalf("completed() len = %d, want 1", len(calls))
	}
	if calls[0].name != "search" {
		t.Errorf("name = %q, want search", calls[0].name)
	}
}
