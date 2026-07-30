package aisdk

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ToolSpec describes a tool the server knows about, for validation purposes.
//
// Executable marks a tool this server dispatches itself. It is the field that decides
// whether a client may supply an output for it — see the note on ValidateUIMessages.
type ToolSpec struct {
	Name string
	// Executable is true for a tool the server runs. False marks a client-side tool,
	// whose result legitimately comes from the browser.
	Executable bool
	// ValidateInput and ValidateOutput are optional schema checks. Left as funcs rather
	// than a schema type so this package stays free of a JSON-schema dependency.
	ValidateInput  func(json.RawMessage) error
	ValidateOutput func(json.RawMessage) error
}

// ToolRegistry looks up tools by name.
type ToolRegistry map[string]ToolSpec

// Lookup resolves a tool name.
//
// The two-value map read is the point. The reference uses a getOwn helper so a toolName
// of "constructor" or "toString" resolves to "no such tool" rather than to a prototype
// member; Go's equivalent hazard is the single-value read, which yields a usable zero
// value and would make an unregistered tool look like a registered one with empty fields.
func (r ToolRegistry) Lookup(name string) (ToolSpec, bool) {
	spec, ok := r[name]
	return spec, ok
}

// ValidateUIMessages checks an inbound history against the protocol and the tool registry.
//
// Ported from ui/validate-ui-messages.ts. Three rules are load-bearing and none is
// obvious:
//
//  1. `input` is validated ONLY for input-available and output-available (:503-517). Not
//     for output-error, and the reference explains why in a comment: a tool call that
//     failed with an invalid-input error keeps that invalid input, and re-validating it on
//     replay throws and "crashes follow-up messages". Since every later turn re-POSTs the
//     whole history, validating it everywhere would let one malformed tool call brick a
//     thread permanently.
//  2. `output` is validated against the tool's output schema for output-available
//     (:519-530).
//  3. Client-supplied output is REJECTED for any server-executable tool. This one is not
//     in the reference and is a deliberate hardening — see below.
//
// The output rule closes a real bypass. The approval HMAC covers `input`; nothing covers
// `output`. Without this rule a client can skip the approval flow entirely by POSTing a
// gated tool part already in state "output-available" with a fabricated output: no
// approval is extracted, no signature is checked, and the model receives a result claiming
// the tool ran. For a tool whose result feeds a decision — checkEntitlement returning
// {tier:"enterprise"} — that steers the next turn's privileged calls.
func ValidateUIMessages(messages []UIMessage, tools ToolRegistry) error {
	for i := range messages {
		m := &messages[i]

		switch m.Role {
		case UIRoleUser, UIRoleAssistant, UIRoleSystem:
		default:
			return &ChatRequestError{
				MessageIndex: i,
				Reason:       fmt.Sprintf("unknown role %q", m.Role),
			}
		}

		for j := range m.Parts {
			if err := validatePart(&m.Parts[j], i, j, tools); err != nil {
				return err
			}
		}
	}
	return nil
}

func validatePart(p *UIMessagePart, msgIdx, partIdx int, tools ToolRegistry) error {
	fail := func(format string, args ...any) error {
		return &ChatRequestError{
			MessageIndex: msgIdx, PartIndex: partIdx,
			Reason: fmt.Sprintf(format, args...),
		}
	}

	if p.IsToolPart() {
		return validateToolPart(p, msgIdx, partIdx, tools)
	}
	if p.IsDataPart() {
		if p.DataNameOf() == "" {
			return fail("data part has an empty name")
		}
		return nil
	}

	switch p.Type {
	case UIPartText, UIPartReasoning:
		return nil
	case UIPartStepStart:
		return nil
	case UIPartFile, UIPartReasoningFile:
		if p.URL == "" {
			return fail("%s part has no url", p.Type)
		}
		if p.MediaType == "" {
			return fail("%s part has no mediaType", p.Type)
		}
		return nil
	case UIPartSourceURL:
		if p.SourceID == "" || p.URL == "" {
			return fail("source-url part needs both sourceId and url")
		}
		return nil
	case UIPartSourceDocument:
		if p.SourceID == "" || p.MediaType == "" || p.Title == "" {
			return fail("source-document part needs sourceId, mediaType and title")
		}
		return nil
	case UIPartCustom:
		if !strings.Contains(p.Kind, ".") {
			return fail("custom.kind %q is not namespaced with a dot", p.Kind)
		}
		return nil
	default:
		return fail("unknown part type %q", p.Type)
	}
}

// validToolStates is the closed set from ui-messages.ts:279-382.
var validToolStates = map[UIToolState]bool{
	UIToolInputStreaming:    true,
	UIToolInputAvailable:    true,
	UIToolApprovalRequested: true,
	UIToolApprovalResponded: true,
	UIToolOutputAvailable:   true,
	UIToolOutputError:       true,
	UIToolOutputDenied:      true,
}

func validateToolPart(p *UIMessagePart, msgIdx, partIdx int, tools ToolRegistry) error {
	fail := func(format string, args ...any) error {
		return &ChatRequestError{
			MessageIndex: msgIdx, PartIndex: partIdx,
			ToolCallID: p.ToolCallID, ToolName: p.ToolNameOf(),
			Reason: fmt.Sprintf(format, args...),
		}
	}

	state := p.ToolStateOf()
	if !validToolStates[state] {
		return fail("unknown tool state %q", state)
	}
	if p.ToolCallID == "" {
		return fail("tool part has an empty toolCallId")
	}
	toolName := p.ToolNameOf()
	if toolName == "" {
		return fail("tool part has no resolvable tool name")
	}

	// Structural checks run BEFORE the registry lookup, because they are protocol
	// invariants rather than facts about a particular tool. Putting them after the
	// unregistered-tool early return would silently skip them for exactly the parts most
	// likely to be malformed.
	switch state {
	case UIToolApprovalRequested, UIToolApprovalResponded, UIToolOutputDenied:
		if p.Approval == nil || p.Approval.ID == "" {
			return fail("state %q requires an approval object with an id", state)
		}
	}
	if state == UIToolApprovalResponded {
		if _, answered := p.ApprovalDecision(); !answered {
			// "Asked but unanswered" and "denied" must not collapse into one value, or
			// the gate would execute a tool nobody approved — or refuse one nobody denied.
			return fail("state approval-responded but approval.approved is absent")
		}
	}

	spec, known := tools.Lookup(toolName)
	if !known {
		// An unregistered tool is not fatal. A history can legitimately contain a tool
		// that has since been removed, and rejecting the whole request would brick the
		// conversation. Dispatch refuses to run it; that is where it matters.
		return nil
	}

	// Client-supplied output for a tool the server executes. Rejected outright: a
	// server-executed tool's result may only come from the server.
	if spec.Executable {
		switch state {
		case UIToolOutputAvailable, UIToolOutputError:
			return fail("state %q is not accepted from the client for the "+
				"server-executed tool %q; its result may only come from this server",
				state, toolName)
		}
	}

	// Input validation, restricted to the two states the reference validates. See the
	// function comment for why output-error is excluded.
	if spec.ValidateInput != nil &&
		(state == UIToolInputAvailable || state == UIToolOutputAvailable) {
		if err := spec.ValidateInput(p.Input); err != nil {
			return &ChatRequestError{
				MessageIndex: msgIdx, PartIndex: partIdx,
				ToolCallID: p.ToolCallID, ToolName: toolName,
				Reason: "input failed the tool's schema", Err: err,
			}
		}
	}

	if spec.ValidateOutput != nil && state == UIToolOutputAvailable {
		if err := spec.ValidateOutput(p.Output); err != nil {
			return &ChatRequestError{
				MessageIndex: msgIdx, PartIndex: partIdx,
				ToolCallID: p.ToolCallID, ToolName: toolName,
				Reason: "output failed the tool's schema", Err: err,
			}
		}
	}

	return nil
}
