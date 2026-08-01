package aisdk

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func TestUseChatApprovalPOST(t *testing.T) {
	raw, err := os.ReadFile("testdata/use_chat_approval_post.json")
	if err != nil {
		t.Fatal(err)
	}
	var envelope ChatRequestEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}

	responses := ApprovalResponses(envelope.Messages)
	if len(responses) != 2 {
		t.Fatalf("approval responses = %#v", responses)
	}
	if !responses[0].Approved || responses[0].ToolName != "deleteFile" || responses[0].Signature != "signed-static" {
		t.Fatalf("static response = %#v", responses[0])
	}
	if responses[1].Approved || responses[1].ToolName != "publish" || responses[1].Reason != "user denied" {
		t.Fatalf("dynamic response = %#v", responses[1])
	}

	messages := ToAIMessages(envelope.Messages)
	if len(messages) != 2 {
		t.Fatalf("messages = %#v, want assistant calls plus user approval responses", messages)
	}
	if messages[0].Role != aikit.RoleAssistant || len(messages[0].Content) != 2 {
		t.Fatalf("assistant message = %#v", messages[0])
	}
	if messages[0].Content[0].ToolApprovalID != "approval-static" ||
		messages[0].Content[1].ToolCallName != "publish" {
		t.Fatalf("assistant tool calls = %#v", messages[0].Content)
	}
	if messages[1].Role != aikit.RoleUser || len(messages[1].Content) != 2 {
		t.Fatalf("approval message = %#v", messages[1])
	}
	if !messages[1].Content[0].ToolApprovalApproved ||
		messages[1].Content[1].ToolApprovalApproved ||
		messages[1].Content[1].ToolApprovalReason != "user denied" {
		t.Fatalf("approval parts = %#v", messages[1].Content)
	}
}

func TestPendingApprovalDoesNotBecomeDenial(t *testing.T) {
	var envelope ChatRequestEnvelope
	if err := json.Unmarshal([]byte(`{"messages":[{"role":"assistant","parts":[{"type":"tool-save","toolCallId":"call-1","state":"approval-requested","input":{},"approval":{"id":"approval-1","signature":"sig"}}]}]}`), &envelope); err != nil {
		t.Fatal(err)
	}
	if responses := ApprovalResponses(envelope.Messages); len(responses) != 0 {
		t.Fatalf("pending approval extracted as response: %#v", responses)
	}
	messages := ToAIMessages(envelope.Messages)
	if len(messages) != 1 || messages[0].Content[0].ToolApprovalID != "approval-1" {
		t.Fatalf("pending messages = %#v", messages)
	}
}
