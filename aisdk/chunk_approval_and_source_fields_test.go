package aisdk

import (
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// TestChunkProducer_ToolApprovalRequest_OptionalFields verifies that the
// tool-approval-request chunk carries isAutomatic and signature only when the
// engine event sets them, matching the protocol's omit-when-absent shape.
func TestChunkProducer_ToolApprovalRequest_OptionalFields(t *testing.T) {
	sr := newEventStream(
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{
			Type:                aikit.StepEventToolApprovalRequest,
			ApprovalID:          "approval-1",
			ToolCallID:          "call-1",
			ToolCallName:        "deleteFile",
			ToolCallArgsDelta:   `{"path":"/tmp/x"}`,
			ApprovalIsAutomatic: true,
			ApprovalSignature:   "sig-abc",
		},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)

	chunks := drainChunks(ToUIMessageStream(sr, "msg-approval", ToUIStreamOptions{}))

	c, ok := findChunk(chunks, ChunkToolApprovalRequest)
	if !ok {
		t.Fatal("expected tool-approval-request chunk")
	}
	if c.Fields["approvalId"] != "approval-1" {
		t.Errorf("approvalId = %v, want approval-1", c.Fields["approvalId"])
	}
	if c.Fields["isAutomatic"] != true {
		t.Errorf("isAutomatic = %v, want true", c.Fields["isAutomatic"])
	}
	if c.Fields["signature"] != "sig-abc" {
		t.Errorf("signature = %v, want sig-abc", c.Fields["signature"])
	}
}

// TestChunkProducer_ToolApprovalRequest_OmitsUnsetOptionalFields verifies the
// optional fields are absent when the engine does not set them.
func TestChunkProducer_ToolApprovalRequest_OmitsUnsetOptionalFields(t *testing.T) {
	sr := newEventStream(
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{
			Type:         aikit.StepEventToolApprovalRequest,
			ToolCallID:   "call-2",
			ToolCallName: "readFile",
		},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)

	chunks := drainChunks(ToUIMessageStream(sr, "msg-approval2", ToUIStreamOptions{}))

	c, ok := findChunk(chunks, ChunkToolApprovalRequest)
	if !ok {
		t.Fatal("expected tool-approval-request chunk")
	}
	if _, has := c.Fields["isAutomatic"]; has {
		t.Error("expected isAutomatic absent when engine does not set it")
	}
	if _, has := c.Fields["signature"]; has {
		t.Error("expected signature absent when engine does not set it")
	}
}

// TestChunkProducer_SourceURL_ProviderMetadata verifies the source-url chunk
// carries providerMetadata when the engine source provides it.
func TestChunkProducer_SourceURL_ProviderMetadata(t *testing.T) {
	pm := map[string]any{"google": map[string]any{"groundingId": "g-1"}}
	sr := newEventStream(
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{
			Type: aikit.StepEventSource,
			Source: &aikit.Source{
				SourceType:       "url",
				ID:               "src-1",
				URL:              "https://example.com",
				Title:            "Example",
				ProviderMetadata: pm,
			},
		},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)

	chunks := drainChunks(ToUIMessageStream(sr, "msg-src", ToUIStreamOptions{SendSources: true}))

	c, ok := findChunk(chunks, ChunkSourceURL)
	if !ok {
		t.Fatal("expected source-url chunk")
	}
	if c.Fields["providerMetadata"] == nil {
		t.Error("expected providerMetadata on source-url chunk when engine provides it")
	}
	if c.Fields["url"] != "https://example.com" {
		t.Errorf("url = %v, want https://example.com", c.Fields["url"])
	}
}
