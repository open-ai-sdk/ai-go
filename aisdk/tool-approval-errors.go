package aisdk

import (
	"errors"
	"fmt"
)

// Sentinels for the approval path, so a caller can classify without matching message
// text. They mirror the reference's error taxonomy: validate-tool-approvals.ts raises
// InvalidToolApprovalSignatureError with a reason, and InvalidToolInputError separately.
var (
	// ErrCanonicalJSON is the base for anything CanonicalJSON refuses to serialize.
	ErrCanonicalJSON = errors.New("aisdk: cannot canonicalize value")

	// ErrInvalidToolApprovalSignature matches the reference's
	// InvalidToolApprovalSignatureError. Carries a reason, since "missing" and "invalid"
	// are distinguishable to the client and to an audit log.
	ErrInvalidToolApprovalSignature = errors.New("aisdk: invalid tool approval signature")

	// ErrInvalidToolInput matches the reference's InvalidToolInputError — the input
	// failed the tool's own schema, which is checked only AFTER the signature.
	ErrInvalidToolInput = errors.New("aisdk: invalid tool input")
)

// Reasons carried by InvalidToolApprovalSignatureError, matching the reference's strings
// so logs and client-visible codes line up across a mixed deployment.
const (
	ApprovalReasonMissingSignature = "missing signature"
	ApprovalReasonInvalidSignature = "invalid signature"
)

// CanonicalJSONError is returned when a value cannot be canonicalized.
type CanonicalJSONError struct {
	Reason string
	// Depth is set for a depth-budget violation.
	Depth int
	Err   error
}

func (e *CanonicalJSONError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("aisdk: cannot canonicalize value: %s: %v", e.Reason, e.Err)
	}
	return fmt.Sprintf("aisdk: cannot canonicalize value: %s", e.Reason)
}

func (e *CanonicalJSONError) Unwrap() error { return ErrCanonicalJSON }

func errCanonicalDepth(depth int) error {
	return &CanonicalJSONError{
		Reason: fmt.Sprintf("nesting exceeds the %d-level budget", CanonicalJSONMaxDepth),
		Depth:  depth,
	}
}

func errCanonicalInvalidUTF8(s string) error {
	// The offending string is deliberately not included: it is attacker-supplied and
	// this error can reach a log.
	return &CanonicalJSONError{
		Reason: fmt.Sprintf("string of %d bytes is not valid UTF-8 (a lone surrogate "+
			"cannot be canonicalized without collapsing distinct values to U+FFFD)", len(s)),
	}
}

func errCanonicalEncode(err error) error {
	return &CanonicalJSONError{Reason: "encoding failed", Err: err}
}

// InvalidToolApprovalSignatureError reports a rejected approval.
//
// It names the approval and tool call so an audit trail can identify what was refused,
// but never echoes the signature — that is attacker-controlled input.
type InvalidToolApprovalSignatureError struct {
	ApprovalID string
	ToolCallID string
	ToolName   string
	Reason     string
}

func (e *InvalidToolApprovalSignatureError) Error() string {
	return fmt.Sprintf("aisdk: invalid tool approval signature: %s "+
		"(approvalId=%q toolCallId=%q toolName=%q)",
		e.Reason, e.ApprovalID, e.ToolCallID, e.ToolName)
}

func (e *InvalidToolApprovalSignatureError) Unwrap() error {
	return ErrInvalidToolApprovalSignature
}

// InvalidToolInputError reports input that failed a tool's schema.
type InvalidToolInputError struct {
	ToolName   string
	ToolCallID string
	Err        error
}

func (e *InvalidToolInputError) Error() string {
	return fmt.Sprintf("aisdk: invalid tool input for %q (toolCallId=%q): %v",
		e.ToolName, e.ToolCallID, e.Err)
}

func (e *InvalidToolInputError) Unwrap() error { return ErrInvalidToolInput }
