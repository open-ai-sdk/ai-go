package aisdk

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrChatRequest classifies anything wrong with an inbound request.
var ErrChatRequest = errors.New("aisdk: invalid chat request")

// ChatRequestError names exactly where a request went wrong.
//
// The indices matter more than they look: this is what a developer sees when their client
// and server disagree about the protocol, and "invalid request" without a message and part
// index means bisecting a conversation by hand.
type ChatRequestError struct {
	MessageIndex int
	PartIndex    int
	ToolCallID   string
	ToolName     string
	Reason       string
	Err          error
}

func (e *ChatRequestError) Error() string {
	var sb strings.Builder
	sb.WriteString("aisdk: invalid chat request")
	if e.MessageIndex > 0 || e.PartIndex > 0 {
		fmt.Fprintf(&sb, " at messages[%d].parts[%d]", e.MessageIndex, e.PartIndex)
	}
	if e.ToolName != "" {
		fmt.Fprintf(&sb, " (tool %q, callId %q)", e.ToolName, e.ToolCallID)
	}
	sb.WriteString(": ")
	sb.WriteString(e.Reason)
	if e.Err != nil {
		fmt.Fprintf(&sb, ": %v", e.Err)
	}
	return sb.String()
}

func (e *ChatRequestError) Unwrap() error { return ErrChatRequest }

// PendingApproval is one approval decision the client returned, extracted for the gate.
//
// Approved comes from the client and is NOT covered by Signature — the signed payload has
// no `approved` term, because the client sets that field itself and passes the signature
// through unchanged. So this struct carries a verified (tool, input) binding next to an
// unverified answer, and the gate must treat the two differently.
type PendingApproval struct {
	ApprovalID string
	ToolCallID string
	ToolName   string
	Input      json.RawMessage
	Approved   bool
	Reason     string
	Signature  string
	// MessageIndex and PartIndex locate the source part, for error reporting.
	MessageIndex int
	PartIndex    int
}

// ExtractPendingApprovals collects the decisions a client answered in this request.
//
// Only approval-responded parts qualify. approval-requested is the server's own question
// coming back unanswered, and output-denied is a decision already carried out — neither is
// something to act on again.
func ExtractPendingApprovals(messages []UIMessage) []PendingApproval {
	var out []PendingApproval
	for i := range messages {
		for j := range messages[i].Parts {
			p := &messages[i].Parts[j]
			if !p.IsToolPart() || p.ToolStateOf() != UIToolApprovalResponded {
				continue
			}
			approved, answered := p.ApprovalDecision()
			if !answered || p.Approval == nil {
				continue
			}
			out = append(out, PendingApproval{
				ApprovalID:   p.Approval.ID,
				ToolCallID:   p.ToolCallID,
				ToolName:     p.ToolNameOf(),
				Input:        p.Input,
				Approved:     approved,
				Reason:       p.Approval.Reason,
				Signature:    p.Approval.Signature,
				MessageIndex: i,
				PartIndex:    j,
			})
		}
	}
	return out
}

// Binding converts a pending approval into the shape the signature covers.
func (p PendingApproval) Binding(principalID, chatID string, issuedAt int64) (ApprovalBinding, error) {
	var input any
	if len(p.Input) > 0 {
		if err := json.Unmarshal(p.Input, &input); err != nil {
			return ApprovalBinding{}, &ChatRequestError{
				MessageIndex: p.MessageIndex, PartIndex: p.PartIndex,
				ToolCallID: p.ToolCallID, ToolName: p.ToolName,
				Reason: "approval input is not valid JSON", Err: err,
			}
		}
	}
	return ApprovalBinding{
		ApprovalID: p.ApprovalID, ToolCallID: p.ToolCallID, ToolName: p.ToolName,
		Input: input, PrincipalID: principalID, ChatID: chatID, IssuedAt: issuedAt,
	}, nil
}

// ExistingToolResults reports tool call ids that already carry a result in this history.
//
// Ported from the reference's existing-result guard (collect-tool-approvals.ts:102-109).
// Without it, an approved call whose result is already in the history executes a second
// time on the next turn — the client keeps re-POSTing the whole conversation, so an
// approval never stops being present.
func ExistingToolResults(messages []UIMessage) map[string]bool {
	out := map[string]bool{}
	for i := range messages {
		for j := range messages[i].Parts {
			p := &messages[i].Parts[j]
			if !p.IsToolPart() {
				continue
			}
			switch p.ToolStateOf() {
			case UIToolOutputAvailable, UIToolOutputError, UIToolOutputDenied:
				out[p.ToolCallID] = true
			}
		}
	}
	return out
}
