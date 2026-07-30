package aisdk

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestIntegration_PersistedMessageBuilder_FullConversation verifies that a realistic
// chunk sequence (text + reasoning + tool + metadata) produces the correct Parts array.
func TestIntegration_PersistedMessageBuilder_FullConversation(t *testing.T) {
	chunks := []Chunk{
		{Type: ChunkStart, Fields: map[string]any{"messageId": "msg-full-1"}},
		{Type: ChunkStartStep, Fields: nil},

		// Reasoning block.
		{Type: ChunkReasoningStart, Fields: map[string]any{"id": "text_1"}},
		{Type: ChunkReasoningDelta, Fields: map[string]any{"id": "text_1", "delta": "Let me think..."}},
		{Type: ChunkReasoningEnd, Fields: map[string]any{"id": "text_1", "signature": "sig-abc"}},

		// Text block.
		{Type: ChunkTextStart, Fields: map[string]any{"id": "text_1"}},
		{Type: ChunkTextDelta, Fields: map[string]any{"id": "text_1", "delta": "Here is my answer."}},
		{Type: ChunkTextEnd, Fields: map[string]any{"id": "text_1"}},

		// Tool invocation (happy path).
		{Type: ChunkToolInputAvailable, Fields: map[string]any{
			"toolCallId": "tc-full-1",
			"toolName":   "calculator",
			"input":      map[string]any{"a": 2, "b": 3},
		}},
		{Type: ChunkToolOutputAvailable, Fields: map[string]any{
			"toolCallId": "tc-full-1",
			"output":     "5",
		}},

		// Message metadata chunk (emitted before finish in the real pipeline).
		{Type: ChunkMessageMetadata, Fields: map[string]any{
			"messageMetadata": map[string]any{"model": "gpt-4o", "totalTokens": 100},
		}},
		{Type: ChunkFinish, Fields: map[string]any{"finishReason": "stop"}},
	}

	builder := NewPersistedMessageBuilder()
	for _, c := range chunks {
		builder.ObserveChunk(c)
	}

	// Verify Content() returns concatenated text.
	if content := builder.Content(); content != "Here is my answer." {
		t.Errorf("Content(): got %q, want %q", content, "Here is my answer.")
	}

	// Verify Parts() contains expected part types.
	raw := builder.Parts()
	if raw == nil {
		t.Fatal("Parts() returned nil")
	}
	var parts []map[string]any
	if err := json.Unmarshal(raw, &parts); err != nil {
		t.Fatalf("Parts() JSON unmarshal: %v", err)
	}

	typeSet := make(map[string]bool)
	for _, p := range parts {
		if typ, ok := p["type"].(string); ok {
			typeSet[typ] = true
		}
	}

	for _, want := range []string{"reasoning", "text", "tool-invocation"} {
		if !typeSet[want] {
			t.Errorf("expected part type %q in parts, got types: %v", want, typeSet)
		}
	}

	// Verify reasoning has signature.
	for _, p := range parts {
		if p["type"] == "reasoning" {
			if sig, ok := p["signature"].(string); !ok || sig != "sig-abc" {
				t.Errorf("reasoning part: expected signature=sig-abc, got %v", p["signature"])
			}
		}
	}

	// Verify tool-invocation has state=output-available and correct toolName.
	for _, p := range parts {
		if p["type"] == "tool-invocation" {
			if p["state"] != "output-available" {
				t.Errorf("tool-invocation state: got %v, want output-available", p["state"])
			}
			if p["toolName"] != "calculator" {
				t.Errorf("tool-invocation toolName: got %v, want calculator", p["toolName"])
			}
		}
	}

	// Verify Metadata() is set from the finish chunk.
	meta := builder.Metadata()
	if meta == nil {
		t.Fatal("Metadata() returned nil, expected message metadata from finish chunk")
	}
	var metaMap map[string]any
	if err := json.Unmarshal(meta, &metaMap); err != nil {
		t.Fatalf("Metadata() JSON unmarshal: %v", err)
	}
	if metaMap["model"] != "gpt-4o" {
		t.Errorf("Metadata model: got %v, want gpt-4o", metaMap["model"])
	}
}

// TestIntegration_ChatRequestRoundTrip_WithAllPartTypes replaces a test written against
// the v6-era ChatRequestEnvelope, which useChat never sent. The scenario is the same — a
// two-message history with text, file, and a completed tool call — retargeted at the real
// v7 body shape.
//
// The old version also asserted ToAIContentParts' fan-out counts. Those move to the
// einoadapter conversion tests, since the target is now *schema.AgenticMessage.
func TestIntegration_ChatRequestRoundTrip_WithAllPartTypes(t *testing.T) {
	body := []byte(`{
	  "id": "sess-rt-1",
	  "trigger": "submit-message",
	  "modelId": "openai:gpt-4o",
	  "maxSteps": 3,
	  "messages": [
	    {"id":"msg-user-1","role":"user","metadata":{"clientTime":"08:00"},"parts":[
	      {"type":"text","text":"What is 2+2?"},
	      {"type":"file","url":"https://example.com/img.png","mediaType":"image/png"}
	    ]},
	    {"id":"msg-asst-1","role":"assistant","parts":[
	      {"type":"step-start"},
	      {"type":"tool-add","toolCallId":"tc-rt-1","state":"output-available",
	       "input":{"a":2,"b":2},"output":"4"},
	      {"type":"text","text":"The answer is 4."}
	    ]}
	  ]
	}`)

	req, err := DecodeChatRequest(bytes.NewReader(body), DefaultDecodeLimits())
	if err != nil {
		t.Fatalf("DecodeChatRequest: %v", err)
	}

	if req.ID != "sess-rt-1" {
		t.Errorf("ID: got %q", req.ID)
	}
	if req.Trigger != TriggerSubmitMessage {
		t.Errorf("Trigger: got %q", req.Trigger)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("messages: got %d, want 2", len(req.Messages))
	}

	// Application fields ride alongside the known keys and stay raw.
	if _, ok := req.Body["modelId"]; !ok {
		t.Errorf("application body fields lost: %v", req.Body)
	}
	for _, known := range []string{"id", "messages", "trigger"} {
		if _, ok := req.Body[known]; ok {
			t.Errorf("protocol key %q leaked into Body", known)
		}
	}

	// Per-message metadata survives as raw JSON — ai-go has no opinion about its shape.
	if len(req.Messages[0].Metadata) == 0 {
		t.Error("per-message metadata was dropped")
	}

	tool := req.Messages[1].Parts[1]
	if !tool.IsToolPart() {
		t.Fatalf("part 1 is not a tool part: %+v", tool)
	}
	if tool.ToolNameOf() != "add" {
		t.Errorf("tool name: got %q, want add", tool.ToolNameOf())
	}
	if tool.ToolCallID != "tc-rt-1" {
		t.Errorf("ToolCallID: got %q", tool.ToolCallID)
	}
	if tool.ToolStateOf() != UIToolOutputAvailable {
		t.Errorf("state: got %q", tool.ToolStateOf())
	}

	// The last message is assistant-role, so the response continues it rather than
	// starting a new one.
	if got := req.ResolveResponseMessageID(); got != "msg-asst-1" {
		t.Errorf("ResolveResponseMessageID = %q, want msg-asst-1", got)
	}
}

// TestIntegration_SendStartFalse_SendFinishFalse_ForMergePattern verifies that
// suppressing both lifecycle chunks leaves only step content, suitable for merging.
func TestIntegration_SendStartFalse_SendFinishFalse_ForMergePattern(t *testing.T) {
	sr := newMockStreamEventer(
		StepEvent{Type: StepEventStepStart},
		StepEvent{Type: StepEventTextDelta, TextDelta: "merged content"},
		StepEvent{Type: StepEventStepEnd, FinishReason: FinishReasonStop},
		StepEvent{Type: StepEventDone},
	)

	ch := ToUIMessageStream(sr, "msg-merge", ToUIStreamOptions{
		SendReasoning: true,
		SendSources:   true,
		SendStart:     boolPtr(false),
		SendFinish:    boolPtr(false),
	})
	chunks := drainChunks(ch)

	if _, ok := findChunk(chunks, ChunkStart); ok {
		t.Error("merge pattern: must not emit start chunk")
	}
	if _, ok := findChunk(chunks, ChunkFinish); ok {
		t.Error("merge pattern: must not emit finish chunk")
	}
	if _, ok := findChunk(chunks, ChunkTextDelta); !ok {
		t.Error("merge pattern: expected text-delta to pass through")
	}
}
