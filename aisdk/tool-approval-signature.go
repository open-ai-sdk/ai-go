package aisdk

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
)

// Domain-separation prefixes. The prefix is the first payload element, so a signature
// produced for one format can never verify against another.
const (
	// ApprovalPayloadV1 is the AI SDK v7 format, from
	// generate-text/tool-approval-signature.ts. Verified for interoperability.
	ApprovalPayloadV1 = "ai-sdk-tool-approval-v1"

	// ApprovalPayloadV2 is ai-go's format, adding the terms v1 lacks. Emitted by default.
	ApprovalPayloadV2 = "ai-go-tool-approval-v2"
)

// ApprovalBinding is what a signature commits to.
//
// What the signature proves, stated precisely so nobody over-reads it: the server gated
// this (tool, input) for this principal, in this chat, at this time. It says NOTHING
// about the user's answer, because `approved` is not in the payload — not in v1, and not
// here either. That is not an oversight carried over from the reference: the client sets
// `approved` itself (ui/chat.ts:477) and spreads the server's signature through unchanged
// (:496), so any value the browser puts there arrives with a valid signature attached.
//
// A user clicks Deny, and anything with client control — XSS, a hostile extension,
// devtools, curl — re-POSTs the same history with approved:true. The signature verifies.
// No signing scheme on this side of the wire can fix that; only a server-side record of
// the decision can, which is what the stateless design trades away.
//
// What the v2 terms DO buy over v1: a signature is bound to one principal, one chat, and
// one issuance time, so it stops being a permanent bearer capability replayable into any
// chat by anyone who obtains it.
type ApprovalBinding struct {
	ApprovalID string
	ToolCallID string
	ToolName   string
	// Input is hashed, not embedded, so a large tool input does not bloat the signature
	// and the digest is what commits to it.
	Input any

	// PrincipalID identifies the caller the approval was issued to. v2 only.
	PrincipalID string
	// ChatID scopes the approval to one conversation. v2 only.
	ChatID string
	// IssuedAt is a Unix timestamp narrowing the replay window. v2 only.
	//
	// Nothing here enforces an expiry — that is a policy decision the verifier makes,
	// because the acceptable window depends on how long a human is given to answer.
	IssuedAt int64
}

// buildPayloadV1 reproduces the reference payload exactly.
//
// JSON array rather than a delimiter join, so the encoding is injective: a field may
// contain any character, including newlines, without making the field boundaries
// ambiguous. The reference also accepts a legacy newline-joined payload as a verify-time
// fallback; ai-go deliberately does not — see VerifyToolApproval.
func buildPayloadV1(b ApprovalBinding, inputDigest string) ([]byte, error) {
	return marshalPayload([]any{
		ApprovalPayloadV1, b.ApprovalID, b.ToolCallID, b.ToolName, inputDigest,
	})
}

func buildPayloadV2(b ApprovalBinding, inputDigest string) ([]byte, error) {
	return marshalPayload([]any{
		ApprovalPayloadV2, b.ApprovalID, b.ToolCallID, b.ToolName, inputDigest,
		b.PrincipalID, b.ChatID, b.IssuedAt,
	})
}

// marshalPayload encodes with HTML escaping off, matching JSON.stringify. A tool name
// containing '&' would otherwise produce different bytes in Go and TypeScript, and the
// signatures would silently stop matching.
func marshalPayload(v []any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, errCanonicalEncode(err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func macBase64URL(secret, payload []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return EncodeBase64URL(mac.Sum(nil))
}

// SignToolApprovalV1 produces a signature in the AI SDK v7 format.
//
// Exported for a deployment that needs a Go server's signatures accepted by a TypeScript
// one. Prefer SignToolApprovalV2 otherwise: v1 binds neither principal nor chat nor time,
// so its signature is a permanent bearer token for one (tool, input) pair.
func SignToolApprovalV1(secret []byte, b ApprovalBinding) (string, error) {
	digest, err := HashCanonical(b.Input)
	if err != nil {
		return "", err
	}
	payload, err := buildPayloadV1(b, digest)
	if err != nil {
		return "", err
	}
	return macBase64URL(secret, payload), nil
}

// SignToolApprovalV2 produces ai-go's default signature, binding principal, chat, and
// issuance time in addition to the v1 terms.
func SignToolApprovalV2(secret []byte, b ApprovalBinding) (string, error) {
	digest, err := HashCanonical(b.Input)
	if err != nil {
		return "", err
	}
	payload, err := buildPayloadV2(b, digest)
	if err != nil {
		return "", err
	}
	return macBase64URL(secret, payload), nil
}

// VerifyToolApproval reports whether signature was issued by this secret for this
// binding. It accepts v2 first, then falls back to v1 so a TypeScript-signed approval
// still verifies.
//
// It returns a bool and not an error on purpose. Every failure mode — a bad MAC, an
// undecodable signature, an input that cannot be canonicalized — means the same thing to
// the caller: this did not verify. Returning an error alongside would invite a caller to
// distinguish cases that must not be distinguished, and a `if err != nil` that forgets to
// also check the bool is an auth bypass.
//
// The legacy newline-joined fallback the reference still accepts is NOT implemented.
// It exists there to verify approvals signed by pre-JSON TypeScript builds; a greenfield
// Go server never issued one, and upstream schedules its removal
// (tool-approval-signature.ts, TODO(#17494): remove in v8). Accepting it would only widen
// what this server trusts.
func VerifyToolApproval(secret []byte, signature string, b ApprovalBinding) bool {
	provided, ok := DecodeBase64URLTolerant(signature)
	if !ok {
		return false
	}

	digest, err := HashCanonical(b.Input)
	if err != nil {
		// An input that cannot be canonicalized cannot have been signed by this server,
		// since signing runs the same code.
		return false
	}

	for _, build := range []func(ApprovalBinding, string) ([]byte, error){
		buildPayloadV2, buildPayloadV1,
	} {
		payload, err := build(b, digest)
		if err != nil {
			continue
		}
		mac := hmac.New(sha256.New, secret)
		mac.Write(payload)
		// hmac.Equal, never ==: string comparison short-circuits on the first differing
		// byte and leaks how much of a forged signature was correct.
		if hmac.Equal(mac.Sum(nil), provided) {
			return true
		}
	}
	return false
}

// CheckToolApproval verifies a signature and returns a typed error on failure, in the
// order validate-tool-approvals.ts fixes: a missing signature is reported separately from
// an invalid one, because the client and an audit log distinguish them.
//
// A nil or empty secret means verification is disabled, matching the reference — and it
// is a real risk, not a convenience: without a secret, any client can forge an approval
// request. Phase 09 makes a configured secret the default with an explicit, logged
// opt-out.
func CheckToolApproval(secret []byte, signature string, b ApprovalBinding) error {
	if len(secret) == 0 {
		return nil
	}
	if signature == "" {
		return &InvalidToolApprovalSignatureError{
			ApprovalID: b.ApprovalID, ToolCallID: b.ToolCallID, ToolName: b.ToolName,
			Reason: ApprovalReasonMissingSignature,
		}
	}
	if !VerifyToolApproval(secret, signature, b) {
		return &InvalidToolApprovalSignatureError{
			ApprovalID: b.ApprovalID, ToolCallID: b.ToolCallID, ToolName: b.ToolName,
			Reason: ApprovalReasonInvalidSignature,
		}
	}
	return nil
}
