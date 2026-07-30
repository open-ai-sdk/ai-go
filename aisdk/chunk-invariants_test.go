package aisdk

import (
	"strings"
	"testing"
)

// Layer 3 of the conformance strategy: the invariants the client cannot detect.
//
// Each case below is a defect that the real ai@7.0.35 client accepts without error and
// then renders incorrectly. A TypeScript negative fixture for any of them would pass,
// which is why they are asserted here instead.

func TestInvariants_CleanSequencePasses(t *testing.T) {
	ok := []Chunk{
		StartChunk("m1"), StartStep(),
		ReasoningStart("r0"), ReasoningDeltaChunk("r0", "think"), ReasoningEnd("r0"),
		TextStart("t0"), TextDeltaChunk("t0", "hi"), TextEnd("t0"),
		ToolInputStart("c1", "echo"), ToolInputDelta("c1", "{"),
		ToolInputAvailable("c1", "echo", map[string]any{}),
		ToolOutputAvailable("c1", "out"),
		FinishStep(), FinishChunk(WireFinishStop),
	}
	if err := CheckChunkInvariants(ok); err != nil {
		t.Errorf("clean sequence rejected: %v", err)
	}
}

// A reused toolCallId is the headline case. getToolInvocation reverse-scans the whole
// message and updateToolPart pushes a duplicate, so the client never throws — it
// silently renders two parts or overwrites one.
func TestInvariants_ReusedToolCallIDIsCaught(t *testing.T) {
	err := CheckChunkInvariants([]Chunk{
		StartChunk("m1"), StartStep(),
		ToolInputStart("c1", "echo"),
		ToolInputAvailable("c1", "echo", map[string]any{}),
		ToolOutputAvailable("c1", "a"),
		ToolInputStart("c1", "echo"), // same id again
		ToolInputAvailable("c1", "echo", map[string]any{}),
		ToolOutputAvailable("c1", "b"),
		FinishStep(), FinishChunk(WireFinishStop),
	})
	if err == nil {
		t.Fatal("reused toolCallId was not caught")
	}
	if !strings.Contains(err.Error(), "reuses toolCallId") {
		t.Errorf("wrong violation: %v", err)
	}
}

// Reuse across a step boundary collides just as badly, since the client's fallback
// lookup spans the whole message rather than the current step.
func TestInvariants_ToolCallIDReuseAcrossStepsIsCaught(t *testing.T) {
	err := CheckChunkInvariants([]Chunk{
		StartChunk("m1"),
		StartStep(), ToolInputStart("c1", "echo"),
		ToolInputAvailable("c1", "echo", map[string]any{}), FinishStep(),
		StartStep(), ToolInputStart("c1", "echo"),
		ToolInputAvailable("c1", "echo", map[string]any{}), FinishStep(),
		FinishChunk(WireFinishStop),
	})
	if err == nil || !strings.Contains(err.Error(), "reuses toolCallId") {
		t.Errorf("cross-step reuse not caught: %v", err)
	}
}

func TestInvariants_EmptyIdentifiersAreCaught(t *testing.T) {
	cases := []struct {
		name  string
		chunk Chunk
		want  string
	}{
		{"empty toolCallId", ToolInputStart("", "echo"), `"toolCallId" is empty`},
		{"empty toolName", ToolInputStart("c1", ""), `"toolName" is empty`},
		{"empty text id", TextStart(""), `"id" is empty`},
		{"empty reasoning id", ReasoningStart(""), `"id" is empty`},
		{"empty approvalId", ToolApprovalRequest("", "c1"), `"approvalId" is empty`},
		{"empty errorText", ErrorChunkText(""), ""}, // constructor substitutes a default
	}
	for _, tc := range cases {
		ic := NewInvariantChecker()
		ic.Observe(tc.chunk)
		err := ic.Err()
		if tc.want == "" {
			if err != nil {
				t.Errorf("%s: unexpected violation %v", tc.name, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s: not caught", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want a violation containing %q", tc.name, err, tc.want)
		}
	}
}

// An unclosed block leaves the client's part at state:"streaming" forever, because
// finish-step resets its active-part maps and nothing can close it afterwards.
func TestInvariants_UnclosedBlockAtFinishStepIsCaught(t *testing.T) {
	err := CheckChunkInvariants([]Chunk{
		StartChunk("m1"), StartStep(),
		TextStart("t0"), TextDeltaChunk("t0", "hi"),
		FinishStep(), // no text-end
		FinishChunk(WireFinishStop),
	})
	if err == nil || !strings.Contains(err.Error(), `text block "t0" still open`) {
		t.Errorf("unclosed text block not caught: %v", err)
	}
}

func TestInvariants_UnclosedReasoningAtFinishStepIsCaught(t *testing.T) {
	err := CheckChunkInvariants([]Chunk{
		StartChunk("m1"), StartStep(),
		ReasoningStart("r0"), ReasoningDeltaChunk("r0", "think"),
		FinishStep(),
		FinishChunk(WireFinishStop),
	})
	if err == nil || !strings.Contains(err.Error(), `reasoning block "r0" still open`) {
		t.Errorf("unclosed reasoning block not caught: %v", err)
	}
}

func TestInvariants_UnterminatedStreamIsCaught(t *testing.T) {
	err := CheckChunkInvariants([]Chunk{
		StartChunk("m1"), StartStep(), TextStart("t0"), TextDeltaChunk("t0", "hi"),
	})
	if err == nil {
		t.Fatal("unterminated stream not caught")
	}
	for _, want := range []string{`text block "t0" still open`, "step still open"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing violation %q in: %v", want, err)
		}
	}
}

// tool-input-available with no input passes the client's schema — verified: input is
// z.unknown(), which accepts an absent key.
func TestInvariants_MissingToolInputIsCaught(t *testing.T) {
	err := CheckChunkInvariants([]Chunk{
		StartChunk("m1"), StartStep(),
		ToolInputStart("c1", "echo"),
		// Built as a raw literal, since the constructor always writes input.
		{Type: ChunkToolInputAvailable, Fields: map[string]any{
			"toolCallId": "c1", "toolName": "echo",
		}},
		FinishStep(), FinishChunk(WireFinishStop),
	})
	if err == nil || !strings.Contains(err.Error(), "has no input field") {
		t.Errorf("missing input not caught: %v", err)
	}
}

// A dotless custom.kind also passes the client: ui-message-chunks.ts declares kind as
// z.string().transform, so the dotted shape is a TypeScript-only constraint.
func TestInvariants_DotlessCustomKindIsCaught(t *testing.T) {
	err := CheckChunkInvariants([]Chunk{
		StartChunk("m1"), StartStep(),
		{Type: ChunkCustom, Fields: map[string]any{"kind": "nodot"}},
		FinishStep(), FinishChunk(WireFinishStop),
	})
	if err == nil || !strings.Contains(err.Error(), "not namespaced with a dot") {
		t.Errorf("dotless custom.kind not caught: %v", err)
	}
}

// The three cases the client DOES throw for are checked here too, so the producer fails
// in Go rather than in the browser.
func TestInvariants_DeltaWithoutStartIsCaught(t *testing.T) {
	cases := []struct {
		name  string
		chunk Chunk
		want  string
	}{
		{"text", TextDeltaChunk("t0", "x"), "text-delta"},
		{"reasoning", ReasoningDeltaChunk("r0", "x"), "reasoning-delta"},
		{"tool input", ToolInputDelta("c1", "{"), "tool-input-delta"},
	}
	for _, tc := range cases {
		ic := NewInvariantChecker()
		ic.Observe(tc.chunk)
		err := ic.Err()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s delta without start not caught: %v", tc.name, err)
		}
	}
}

func TestInvariants_DuplicateStartForSameIDIsCaught(t *testing.T) {
	err := CheckChunkInvariants([]Chunk{
		StartChunk("m1"), StartStep(),
		TextStart("t0"), TextStart("t0"),
		TextEnd("t0"), FinishStep(), FinishChunk(WireFinishStop),
	})
	if err == nil || !strings.Contains(err.Error(), "already open") {
		t.Errorf("duplicate text-start not caught: %v", err)
	}
}

// A provider-executed tool sends tool-input-available with no preceding
// tool-input-start, because its arguments do not stream. That must stay legal.
func TestInvariants_ProviderExecutedToolWithoutInputStartIsAllowed(t *testing.T) {
	err := CheckChunkInvariants([]Chunk{
		StartChunk("m1"), StartStep(),
		ToolInputAvailable("c2", "web_search", map[string]any{"q": "x"},
			WithProviderExecuted(true)),
		ToolOutputAvailable("c2", "3 results", WithProviderExecuted(true)),
		FinishStep(), FinishChunk(WireFinishStop),
	})
	if err != nil {
		t.Errorf("provider-executed tool rejected: %v", err)
	}
}

// Every committed conformance fixture must satisfy layer 3 as well, except the ones
// deliberately built to violate it.
func TestInvariants_AllPositiveFixturesAreClean(t *testing.T) {
	for _, f := range conformanceFixtures() {
		if f.dir != "" {
			continue
		}
		// The error fixture ends mid-stream on purpose: the run failed, so the text
		// block is legitimately never closed and no finish-step arrives.
		if f.name == "error" {
			continue
		}
		if err := CheckChunkInvariants(f.chunks); err != nil {
			t.Errorf("fixture %s violates a producer invariant: %v", f.name, err)
		}
	}
}

// And the negative fixtures must actually violate it, so layer 3 and the TS layers agree
// on which streams are malformed.
func TestInvariants_NegativeFixturesAreRejected(t *testing.T) {
	for _, f := range conformanceFixtures() {
		if f.dir != "invalid" {
			continue
		}
		if err := CheckChunkInvariants(f.chunks); err == nil {
			t.Errorf("negative fixture %s passes the invariant checker; layer 3 and the "+
				"TS processor disagree about whether it is malformed", f.name)
		}
	}
}
