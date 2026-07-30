package aisdk

import (
	"fmt"
	"sort"
	"strings"
)

// InvariantChecker validates a chunk sequence against the rules the AI SDK v7 client
// does NOT enforce.
//
// The client is strict in two places and quietly lenient everywhere else. It throws for
// a `*-delta` without its `*-start`, and its Zod gate rejects an unknown chunk type. But
// a reused or empty toolCallId does not throw — `getToolInvocation` reverse-scans the
// whole message before giving up, and `updateToolPart` will happily push a duplicate
// part. An unclosed text block is not an error either. Those defects render *wrong*
// rather than failing, which is far harder to diagnose than an exception.
//
// So they have to be caught on the producing side. This type is that check, kept
// separate from the producer so it can wrap any chunk source.
type InvariantChecker struct {
	activeText      map[string]bool
	activeReasoning map[string]bool
	toolStarted     map[string]bool
	// seenToolCallIDs spans the whole message, not the step: the client's fallback
	// lookup scans the entire message, so a reuse across steps collides just as badly
	// as one within a step.
	seenToolCallIDs map[string]bool
	// seenApprovalIDs mirrors the client, which resolves a tool-approval-response by
	// looking up the id its request registered.
	seenApprovalIDs map[string]bool
	stepOpen        bool
	violations      []string
}

func NewInvariantChecker() *InvariantChecker {
	return &InvariantChecker{
		activeText:      map[string]bool{},
		activeReasoning: map[string]bool{},
		toolStarted:     map[string]bool{},
		seenToolCallIDs: map[string]bool{},
		seenApprovalIDs: map[string]bool{},
	}
}

func (ic *InvariantChecker) failf(format string, args ...any) {
	ic.violations = append(ic.violations, fmt.Sprintf(format, args...))
}

// str reads a required string field, recording a violation when it is absent or empty.
func (ic *InvariantChecker) str(c Chunk, key string) string {
	v, ok := c.Fields[key]
	if !ok {
		ic.failf("%s: missing required field %q", c.Type, key)
		return ""
	}
	s, isStr := v.(string)
	if !isStr {
		ic.failf("%s: field %q is %T, want string", c.Type, key, v)
		return ""
	}
	if s == "" {
		// The client accepts an empty id and then cannot match anything to it.
		ic.failf("%s: field %q is empty", c.Type, key)
	}
	return s
}

// Observe feeds one chunk through the checker.
func (ic *InvariantChecker) Observe(c Chunk) {
	switch c.Type {
	case ChunkStartStep:
		ic.stepOpen = true

	case ChunkFinishStep:
		// finish-step resets the client's active text/reasoning maps, so anything still
		// open here can never be closed: its text part stays state:"streaming" forever.
		for _, id := range sortedKeys(ic.activeText) {
			ic.failf("finish-step with text block %q still open", id)
		}
		for _, id := range sortedKeys(ic.activeReasoning) {
			ic.failf("finish-step with reasoning block %q still open", id)
		}
		ic.activeText = map[string]bool{}
		ic.activeReasoning = map[string]bool{}
		ic.stepOpen = false

	case ChunkTextStart:
		id := ic.str(c, "id")
		if ic.activeText[id] {
			ic.failf("text-start for %q while that block is already open", id)
		}
		ic.activeText[id] = true

	case ChunkTextDelta:
		id := ic.str(c, "id")
		ic.str(c, "delta")
		if !ic.activeText[id] {
			ic.failf("text-delta for %q with no open text-start (the client throws)", id)
		}

	case ChunkTextEnd:
		id := ic.str(c, "id")
		if !ic.activeText[id] {
			ic.failf("text-end for %q with no open text-start (the client throws)", id)
		}
		delete(ic.activeText, id)

	case ChunkReasoningStart:
		id := ic.str(c, "id")
		if ic.activeReasoning[id] {
			ic.failf("reasoning-start for %q while that block is already open", id)
		}
		ic.activeReasoning[id] = true

	case ChunkReasoningDelta:
		id := ic.str(c, "id")
		ic.str(c, "delta")
		if !ic.activeReasoning[id] {
			ic.failf("reasoning-delta for %q with no open reasoning-start (the client throws)", id)
		}

	case ChunkReasoningEnd:
		id := ic.str(c, "id")
		if !ic.activeReasoning[id] {
			ic.failf("reasoning-end for %q with no open reasoning-start (the client throws)", id)
		}
		delete(ic.activeReasoning, id)

	case ChunkToolInputStart:
		id := ic.str(c, "toolCallId")
		ic.str(c, "toolName")
		ic.noteToolCallID(c.Type, id)
		ic.toolStarted[id] = true

	case ChunkToolInputDelta:
		id := ic.str(c, "toolCallId")
		ic.str(c, "inputTextDelta")
		if !ic.toolStarted[id] {
			ic.failf("tool-input-delta for %q with no tool-input-start (the client throws)", id)
		}

	case ChunkToolInputAvailable:
		id := ic.str(c, "toolCallId")
		ic.str(c, "toolName")
		// The schema types input as z.unknown(), so an absent input validates. But a
		// consumer of persisted history cannot tell "no input" from "field dropped",
		// which makes it a producer bug the client will never report.
		if _, ok := c.Fields["input"]; !ok {
			ic.failf("tool-input-available for %q has no input field", id)
		}
		if !ic.toolStarted[id] {
			// Legitimate for a provider-executed tool, whose arguments do not stream.
			ic.noteToolCallID(c.Type, id)
		}

	case ChunkToolInputError:
		ic.str(c, "toolCallId")
		ic.str(c, "toolName")
		ic.str(c, "errorText")

	case ChunkToolOutputAvailable:
		id := ic.str(c, "toolCallId")
		if _, ok := c.Fields["output"]; !ok {
			ic.failf("tool-output-available for %q has no output field", id)
		}

	case ChunkToolOutputError:
		ic.str(c, "toolCallId")
		ic.str(c, "errorText")

	case ChunkToolOutputDenied:
		ic.str(c, "toolCallId")

	case ChunkToolApprovalRequest:
		id := ic.str(c, "approvalId")
		ic.str(c, "toolCallId")
		if id != "" {
			if ic.seenApprovalIDs[id] {
				ic.failf("tool-approval-request reuses approvalId %q", id)
			}
			ic.seenApprovalIDs[id] = true
		}

	case ChunkToolApprovalResponse:
		id := ic.str(c, "approvalId")
		// One of the client's seven throw sites: an approvalId with no matching
		// invocation raises "No tool invocation found for approval ID". Catching it here
		// keeps layer 3 in agreement with the processor about which streams are
		// malformed, which the cross-check test enforces.
		if id != "" && !ic.seenApprovalIDs[id] {
			ic.failf("tool-approval-response for unknown approvalId %q "+
				"(no preceding tool-approval-request; the client throws)", id)
		}

	case ChunkError:
		ic.str(c, "errorText")

	case ChunkCustom:
		// Enforced here as well as in the constructor, since a raw Chunk literal can
		// bypass the constructor. The client accepts a dotless kind.
		if kind := ic.str(c, "kind"); kind != "" && !strings.Contains(kind, ".") {
			ic.failf("custom.kind %q is not namespaced with a dot", kind)
		}
	}
}

// noteToolCallID records a call id and flags reuse within the message.
func (ic *InvariantChecker) noteToolCallID(chunkType, id string) {
	if id == "" {
		return // already reported by str
	}
	if ic.seenToolCallIDs[id] {
		ic.failf("%s reuses toolCallId %q within one message; the client does not "+
			"detect this and will overwrite or duplicate the earlier part", chunkType, id)
	}
	ic.seenToolCallIDs[id] = true
}

// Finish reports any block left open when the stream ended.
func (ic *InvariantChecker) Finish() {
	for _, id := range sortedKeys(ic.activeText) {
		ic.failf("stream ended with text block %q still open", id)
	}
	for _, id := range sortedKeys(ic.activeReasoning) {
		ic.failf("stream ended with reasoning block %q still open", id)
	}
	if ic.stepOpen {
		ic.failf("stream ended with a step still open (no finish-step)")
	}
}

// Violations returns every rule broken so far, in the order detected.
func (ic *InvariantChecker) Violations() []string { return ic.violations }

// Err returns nil when the sequence is clean, or an error naming every violation.
func (ic *InvariantChecker) Err() error {
	if len(ic.violations) == 0 {
		return nil
	}
	return fmt.Errorf("chunk invariant violations:\n  - %s",
		strings.Join(ic.violations, "\n  - "))
}

// CheckChunkInvariants runs a complete sequence through a fresh checker.
func CheckChunkInvariants(chunks []Chunk) error {
	ic := NewInvariantChecker()
	for _, c := range chunks {
		ic.Observe(c)
	}
	ic.Finish()
	return ic.Err()
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
