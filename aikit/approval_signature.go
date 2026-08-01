package aikit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidToolApprovalSignature = errors.New("aikit: invalid tool approval signature")

// ToolApprovalSignatureInput identifies the server-gated tool capability.
type ToolApprovalSignatureInput struct {
	ApprovalID string
	ToolCallID string
	ToolName   string
	Input      json.RawMessage
}

// SignToolApproval signs the AI SDK v7 payload
// ["ai-sdk-tool-approval-v1", approvalId, toolCallId, toolName,
// hashCanonical(input)]. Approved is intentionally absent: the signature
// proves that the server gated this tool and input, not that the user approved.
// A compromised client can still choose to approve a valid request.
func SignToolApproval(secret []byte, input ToolApprovalSignatureInput) (string, error) {
	if len(secret) == 0 {
		return "", errors.New("aisdk: tool approval secret is empty")
	}
	digest, err := HashCanonicalToolApprovalInput(input.Input)
	if err != nil {
		return "", fmt.Errorf("aisdk: canonicalize tool approval input: %w", err)
	}
	payload := "[" + strings.Join([]string{
		approvalJSONString("ai-sdk-tool-approval-v1"), approvalJSONString(input.ApprovalID),
		approvalJSONString(input.ToolCallID), approvalJSONString(input.ToolName), approvalJSONString(digest),
	}, ",") + "]"
	return approvalMAC(secret, []byte(payload)), nil
}

// CanonicalizeToolApprovalInput returns the exact JSON bytes hashed by the
// approval signature contract. Runtime packages use this when they need to
// bind a verified capability to normalized tool arguments.
func CanonicalizeToolApprovalInput(input json.RawMessage) ([]byte, error) {
	canonical, err := canonicalApprovalJSON(input)
	if err != nil {
		return nil, fmt.Errorf("aisdk: canonicalize tool approval input: %w", err)
	}
	return canonical, nil
}

// VerifyToolApproval verifies the current injective JSON-array payload. For
// migration from early v7 clients it also accepts the legacy newline payload,
// but only when the identifying fields contain no newline ambiguity.
func VerifyToolApproval(secret []byte, signature string, input ToolApprovalSignatureInput) error {
	expected, err := SignToolApproval(secret, input)
	if err != nil {
		return err
	}
	if equalApprovalSignature(expected, signature) {
		return nil
	}
	if strings.ContainsAny(input.ApprovalID+input.ToolCallID+input.ToolName, "\n") {
		return ErrInvalidToolApprovalSignature
	}
	digest, err := HashCanonicalToolApprovalInput(input.Input)
	if err != nil {
		return err
	}
	legacy := strings.Join([]string{
		input.ApprovalID, input.ToolCallID, input.ToolName, digest,
	}, "\n")
	if equalApprovalSignature(approvalMAC(secret, []byte(legacy)), signature) {
		return nil
	}
	return ErrInvalidToolApprovalSignature
}

func approvalMAC(secret, payload []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func equalApprovalSignature(expected, actual string) bool {
	expectedBytes, err := base64.RawURLEncoding.DecodeString(expected)
	if err != nil {
		return false
	}
	actualBytes, err := base64.RawURLEncoding.DecodeString(actual)
	return err == nil && hmac.Equal(expectedBytes, actualBytes)
}
