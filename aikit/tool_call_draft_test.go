package aikit

import "testing"

func toolCallDelta(index int, id, name, argsDelta string) StreamEvent {
	return StreamEvent{
		Type:              StreamEventToolCallDelta,
		ToolCallIndex:     index,
		ToolCallID:        id,
		ToolCallName:      name,
		ToolCallArgsDelta: argsDelta,
	}
}

func TestToolCallFoldAssemblesFragmentedArguments(t *testing.T) {
	var fold ToolCallFold
	if isNew := fold.Add(toolCallDelta(0, "tc1", "search", `{"q`)); !isNew {
		t.Fatal("Add() isNew = false on the first delta, want true")
	}
	if isNew := fold.Add(toolCallDelta(0, "tc1", "search", `":"hel`)); isNew {
		t.Fatal("Add() isNew = true on a continuation delta, want false")
	}
	fold.Add(toolCallDelta(0, "tc1", "search", `lo"}`))

	drafts := fold.Completed()
	if len(drafts) != 1 {
		t.Fatalf("Completed() len = %d, want 1", len(drafts))
	}
	if drafts[0].Args != `{"q":"hello"}` {
		t.Errorf("Args = %q, want %q", drafts[0].Args, `{"q":"hello"}`)
	}
	if !drafts[0].Complete {
		t.Error("Complete = false after the arguments closed, want true")
	}
}

func TestToolCallFoldSingleCompleteDelta(t *testing.T) {
	var fold ToolCallFold
	fold.Add(toolCallDelta(0, "tc1", "get_time", `{"tz":"UTC"}`))

	drafts := fold.Completed()
	if len(drafts) != 1 {
		t.Fatalf("Completed() len = %d, want 1", len(drafts))
	}
	if drafts[0].Args != `{"tz":"UTC"}` {
		t.Errorf("Args = %q, want %q", drafts[0].Args, `{"tz":"UTC"}`)
	}
	if !drafts[0].Complete {
		t.Error("Complete = false, want true")
	}
}

// A provider that re-sends complete arguments must not have them concatenated
// into invalid JSON. This is the canonical behavior direct completions gained
// when the two folds were unified.
func TestToolCallFoldIgnoresDeltasAfterValidJSON(t *testing.T) {
	var fold ToolCallFold
	fold.Add(toolCallDelta(0, "tc1", "echo", `{}`))
	fold.Add(toolCallDelta(0, "tc1", "echo", `,"extra":true`))
	fold.Add(toolCallDelta(0, "tc1", "echo", `}`))

	drafts := fold.Completed()
	if len(drafts) != 1 {
		t.Fatalf("Completed() len = %d, want 1", len(drafts))
	}
	if drafts[0].Args != `{}` {
		t.Errorf("Args = %q, want %q", drafts[0].Args, `{}`)
	}
}

func TestToolCallFoldKeepsIncompleteArgumentsAsIs(t *testing.T) {
	var fold ToolCallFold
	fold.Add(toolCallDelta(0, "tc1", "broken", `{"partial`))

	drafts := fold.Completed()
	if len(drafts) != 1 {
		t.Fatalf("Completed() len = %d, want 1", len(drafts))
	}
	if drafts[0].Args != `{"partial` {
		t.Errorf("Args = %q, want %q", drafts[0].Args, `{"partial`)
	}
	if drafts[0].Complete {
		t.Error("Complete = true for unterminated JSON, want false")
	}
}

func TestToolCallFoldSeparatesConcurrentIndexes(t *testing.T) {
	var fold ToolCallFold
	fold.Add(toolCallDelta(0, "tc1", "search", `{"q`))
	fold.Add(toolCallDelta(1, "tc2", "fetch", `{"url`))
	fold.Add(toolCallDelta(0, "tc1", "search", `":"a"}`))
	fold.Add(toolCallDelta(1, "tc2", "fetch", `":"b"}`))

	drafts := fold.Completed()
	if len(drafts) != 2 {
		t.Fatalf("Completed() len = %d, want 2", len(drafts))
	}
	if drafts[0].Args != `{"q":"a"}` {
		t.Errorf("index 0 Args = %q, want %q", drafts[0].Args, `{"q":"a"}`)
	}
	if drafts[1].Args != `{"url":"b"}` {
		t.Errorf("index 1 Args = %q, want %q", drafts[1].Args, `{"url":"b"}`)
	}
	if !drafts[0].Complete || !drafts[1].Complete {
		t.Error("Complete = false for a closed argument object, want true for both")
	}
}

func TestToolCallFoldAcceptsEmptyArguments(t *testing.T) {
	var fold ToolCallFold
	fold.Add(toolCallDelta(0, "tc1", "noop", ""))

	drafts := fold.Completed()
	if len(drafts) != 1 {
		t.Fatalf("Completed() len = %d, want 1", len(drafts))
	}
	if drafts[0].Args != "" {
		t.Errorf("Args = %q, want empty", drafts[0].Args)
	}
	if drafts[0].Complete {
		t.Error("Complete = true for absent arguments, want false")
	}
}

// Anthropic's OpenAI-compatible API starts tool-call indexes at 1.
func TestToolCallFoldOrdersByProviderIndex(t *testing.T) {
	var fold ToolCallFold
	fold.Add(toolCallDelta(3, "tc3", "third", `{}`))
	fold.Add(toolCallDelta(1, "tc1", "bash", `{"command":"ls"}`))

	drafts := fold.Completed()
	if len(drafts) != 2 {
		t.Fatalf("Completed() len = %d, want 2", len(drafts))
	}
	if drafts[0].Index != 1 || drafts[0].Name != "bash" {
		t.Errorf("first draft = index %d name %q, want index 1 name bash", drafts[0].Index, drafts[0].Name)
	}
	if drafts[1].Index != 3 {
		t.Errorf("second draft index = %d, want 3", drafts[1].Index)
	}
}

// Behavior the agent gained when the folds were unified: an OpenAI-compatible
// provider may send the tool name only on a later chunk, and losing it fails
// the call.
func TestToolCallFoldOverwritesLateIdentityFields(t *testing.T) {
	var fold ToolCallFold
	fold.Add(toolCallDelta(0, "", "", `{"a`))
	fold.Add(toolCallDelta(0, "tc1", "search", `":1}`))

	drafts := fold.Completed()
	if len(drafts) != 1 {
		t.Fatalf("Completed() len = %d, want 1", len(drafts))
	}
	if drafts[0].ID != "tc1" {
		t.Errorf("ID = %q, want tc1", drafts[0].ID)
	}
	if drafts[0].Name != "search" {
		t.Errorf("Name = %q, want search", drafts[0].Name)
	}
}

func TestToolCallFoldKeepsIdentityWhenLaterDeltasOmitIt(t *testing.T) {
	var fold ToolCallFold
	fold.Add(toolCallDelta(0, "tc1", "search", `{"a`))
	fold.Add(toolCallDelta(0, "", "", `":1}`))

	drafts := fold.Completed()
	if drafts[0].ID != "tc1" || drafts[0].Name != "search" {
		t.Errorf("draft identity = (%q, %q), want (tc1, search)", drafts[0].ID, drafts[0].Name)
	}
}

// The signature belongs to the call as announced, so the first non-empty value
// wins and a later disagreeing one is ignored.
func TestToolCallFoldKeepsFirstNonEmptyThoughtSignature(t *testing.T) {
	var fold ToolCallFold
	first := toolCallDelta(0, "tc1", "search", `{"a`)
	fold.Add(first)

	second := toolCallDelta(0, "tc1", "search", `":1}`)
	second.ThoughtSignature = "sig-late"
	fold.Add(second)

	if got := fold.Completed()[0].ThoughtSignature; got != "sig-late" {
		t.Fatalf("ThoughtSignature = %q, want sig-late adopted when the first delta had none", got)
	}
}

func TestToolCallFoldIgnoresConflictingThoughtSignature(t *testing.T) {
	var fold ToolCallFold
	first := toolCallDelta(0, "tc1", "search", `{"a`)
	first.ThoughtSignature = "sig-first"
	fold.Add(first)

	later := toolCallDelta(0, "tc1", "search", `":1}`)
	later.ThoughtSignature = "sig-other"
	fold.Add(later)

	if got := fold.Completed()[0].ThoughtSignature; got != "sig-first" {
		t.Errorf("ThoughtSignature = %q, want sig-first", got)
	}
}

func TestToolCallFoldZeroValueIsUsable(t *testing.T) {
	var fold ToolCallFold
	if fold.Len() != 0 {
		t.Errorf("Len() = %d on a zero fold, want 0", fold.Len())
	}
	if fold.Completed() != nil {
		t.Error("Completed() on a zero fold is non-nil, want nil")
	}
	fold.Add(toolCallDelta(0, "tc1", "noop", ""))
	if fold.Len() != 1 {
		t.Errorf("Len() = %d after one delta, want 1", fold.Len())
	}
}
