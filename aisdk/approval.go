package aisdk

import (
	"encoding/json"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// ErrInvalidToolApprovalSignature reports an invalid approval capability.
var ErrInvalidToolApprovalSignature = aikit.ErrInvalidToolApprovalSignature

// ToolApprovalSignatureInput identifies the server-gated tool capability.
type ToolApprovalSignatureInput = aikit.ToolApprovalSignatureInput

// SignToolApproval signs the AI SDK v7 approval payload.
func SignToolApproval(secret []byte, input ToolApprovalSignatureInput) (string, error) {
	return aikit.SignToolApproval(secret, input)
}

// VerifyToolApproval verifies an AI SDK v7 approval signature.
func VerifyToolApproval(secret []byte, signature string, input ToolApprovalSignatureInput) error {
	return aikit.VerifyToolApproval(secret, signature, input)
}

// CanonicalizeToolApprovalInput returns the exact canonical JSON bytes bound
// by an approval signature.
func CanonicalizeToolApprovalInput(input json.RawMessage) ([]byte, error) {
	return aikit.CanonicalizeToolApprovalInput(input)
}

// HashCanonical hashes JSON using the approval contract's JavaScript-compatible
// canonical representation.
func HashCanonical(input json.RawMessage) (string, error) {
	return aikit.HashCanonicalToolApprovalInput(input)
}
