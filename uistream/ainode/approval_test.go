package ainode

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
)

type approvalVector struct {
	Name, Secret, ApprovalID, ToolCallID, ToolName string
	Input                                          json.RawMessage
	Digest, Signature, LegacySignature             string
}

func loadApprovalVectors(t *testing.T) []approvalVector {
	t.Helper()
	raw, err := os.ReadFile("testdata/tool_approval_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []approvalVector
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	return vectors
}

func TestToolApprovalSignaturesMatchNodeVectors(t *testing.T) {
	for _, vector := range loadApprovalVectors(t) {
		t.Run(vector.Name, func(t *testing.T) {
			input := ToolApprovalSignatureInput{
				ApprovalID: vector.ApprovalID, ToolCallID: vector.ToolCallID,
				ToolName: vector.ToolName, Input: vector.Input,
			}
			digest, err := HashCanonical(vector.Input)
			if err != nil {
				t.Fatal(err)
			}
			if digest != vector.Digest {
				t.Fatalf("digest = %q, want %q", digest, vector.Digest)
			}
			signature, err := SignToolApproval([]byte(vector.Secret), input)
			if err != nil {
				t.Fatal(err)
			}
			if signature != vector.Signature {
				t.Fatalf("signature = %q, want %q", signature, vector.Signature)
			}
			if err := VerifyToolApproval([]byte(vector.Secret), signature, input); err != nil {
				t.Fatal(err)
			}
			if vector.LegacySignature != "" {
				if err := VerifyToolApproval([]byte(vector.Secret), vector.LegacySignature, input); err != nil {
					t.Fatalf("legacy migration signature: %v", err)
				}
			}
			if err := VerifyToolApproval(
				[]byte(vector.Secret),
				signature+"x",
				input,
			); !errors.Is(
				err,
				ErrInvalidToolApprovalSignature,
			) {
				t.Fatalf("tampered signature error = %v", err)
			}
		})
	}
}

func TestToolApprovalRejectsAmbiguousLegacyPayload(t *testing.T) {
	vector := loadApprovalVectors(t)[0]
	input := ToolApprovalSignatureInput{
		ApprovalID: vector.ApprovalID + "\nforged", ToolCallID: vector.ToolCallID,
		ToolName: vector.ToolName, Input: vector.Input,
	}
	if err := VerifyToolApproval(
		[]byte(vector.Secret),
		vector.LegacySignature,
		input,
	); !errors.Is(
		err,
		ErrInvalidToolApprovalSignature,
	) {
		t.Fatalf("ambiguous legacy signature error = %v", err)
	}
}
