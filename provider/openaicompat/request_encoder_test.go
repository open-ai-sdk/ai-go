package openaicompat

import (
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
)

func TestEncodeRequest_StopSequences(t *testing.T) {
	req := ai.LanguageModelRequest{
		Messages: []ai.Message{ai.UserMessage("hello")},
		Settings: ai.CallSettings{
			StopSequences: []string{"<END>", "STOP"},
		},
	}

	cr, _, err := EncodeRequest(EncodeRequestParams{ModelID: "test"}, req, true)
	if err != nil {
		t.Fatalf("EncodeRequest failed: %v", err)
	}

	if len(cr.Stop) != 2 {
		t.Fatalf("expected 2 stop sequences, got %d", len(cr.Stop))
	}
	if cr.Stop[0] != "<END>" || cr.Stop[1] != "STOP" {
		t.Errorf("unexpected stop sequences: %v", cr.Stop)
	}
}

func TestEncodeRequest_StopSequences_Empty(t *testing.T) {
	req := ai.LanguageModelRequest{
		Messages: []ai.Message{ai.UserMessage("hello")},
	}

	cr, _, err := EncodeRequest(EncodeRequestParams{ModelID: "test"}, req, true)
	if err != nil {
		t.Fatalf("EncodeRequest failed: %v", err)
	}

	if len(cr.Stop) != 0 {
		t.Errorf("expected no stop sequences, got %v", cr.Stop)
	}
}

func TestEncodeRequest_AssistantToolCallOnly_HasContentNull(t *testing.T) {
	// Assistant message with only a tool call and no text content.
	msg := ai.Message{
		Role: ai.RoleAssistant,
		Content: []ai.ContentPart{
			{
				Type:         ai.ContentPartTypeToolCall,
				ToolCallID:   "tc1",
				ToolCallName: "search",
				ToolCallArgs: []byte(`{"q":"test"}`),
			},
		},
	}

	req := ai.LanguageModelRequest{
		Messages: []ai.Message{ai.UserMessage("hello"), msg},
	}

	cr, _, err := EncodeRequest(EncodeRequestParams{ModelID: "test"}, req, true)
	if err != nil {
		t.Fatalf("EncodeRequest failed: %v", err)
	}

	// Find the assistant message (second message after system).
	var assistantMsg map[string]any
	for _, m := range cr.Messages {
		if m["role"] == "assistant" {
			assistantMsg = m
			break
		}
	}
	if assistantMsg == nil {
		t.Fatal("assistant message not found")
	}

	// content key must exist.
	contentVal, exists := assistantMsg["content"]
	if !exists {
		t.Fatal("expected content key to exist in assistant message")
	}
	// content must be nil (JSON null).
	if contentVal != nil {
		t.Errorf("expected content to be nil, got %v", contentVal)
	}
	// tool_calls must exist.
	if _, exists := assistantMsg["tool_calls"]; !exists {
		t.Fatal("expected tool_calls key in assistant message")
	}
}

func TestEncodeRequest_AssistantWithText_HasContentParts(t *testing.T) {
	msg := ai.Message{
		Role: ai.RoleAssistant,
		Content: []ai.ContentPart{
			{Type: ai.ContentPartTypeText, Text: "thinking..."},
			{
				Type:         ai.ContentPartTypeToolCall,
				ToolCallID:   "tc1",
				ToolCallName: "search",
				ToolCallArgs: []byte(`{"q":"test"}`),
			},
		},
	}

	req := ai.LanguageModelRequest{
		Messages: []ai.Message{ai.UserMessage("hello"), msg},
	}

	cr, _, err := EncodeRequest(EncodeRequestParams{ModelID: "test"}, req, true)
	if err != nil {
		t.Fatalf("EncodeRequest failed: %v", err)
	}

	var assistantMsg map[string]any
	for _, m := range cr.Messages {
		if m["role"] == "assistant" {
			assistantMsg = m
			break
		}
	}
	if assistantMsg == nil {
		t.Fatal("assistant message not found")
	}

	content, ok := assistantMsg["content"]
	if !ok || content == nil {
		t.Fatal("expected non-nil content for assistant message with text")
	}
}

func TestEncodeRequest_DocumentUsesFileWarningPath(t *testing.T) {
	req := ai.LanguageModelRequest{Messages: []ai.Message{{
		Role: ai.RoleUser,
		Content: []ai.ContentPart{
			ai.TextPart("summarize"),
			ai.DocumentFileIDPart("file-document", "application/pdf"),
		},
	}}}

	encoded, warnings, err := EncodeRequest(EncodeRequestParams{ModelID: "test"}, req, false)
	if err != nil {
		t.Fatalf("EncodeRequest() error = %v", err)
	}
	if len(warnings) != 1 || warnings[0].Setting != "fileID" {
		t.Fatalf("warnings = %#v, want fileID omission warning", warnings)
	}
	if len(encoded.Messages) != 1 {
		t.Fatalf("messages = %#v", encoded.Messages)
	}
	content, ok := encoded.Messages[0]["content"].([]map[string]any)
	if !ok || len(content) != 1 || content[0]["text"] != "summarize" {
		t.Fatalf("content = %#v, want only retained text part", encoded.Messages[0]["content"])
	}
}
