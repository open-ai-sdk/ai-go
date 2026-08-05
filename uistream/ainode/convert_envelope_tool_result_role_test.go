// Pin that a decoded conversation is valid input for a run. The decoder and
// aikit's message validation are two halves of the same contract: a history
// this package produces must be a history the agent accepts.
package ainode

import (
	"encoding/json"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// completedToolTurn is what useChat posts back after a tool call resolves.
func completedToolTurn() []EnvelopeMessage {
	return []EnvelopeMessage{
		{Role: "user", Parts: []EnvelopePartUnion{{Type: EnvelopePartTypeText, Text: "weather?"}}},
		{Role: "assistant", Parts: []EnvelopePartUnion{
			{
				Type: "tool-get_weather", ToolCallID: "call_1", ToolName: "get_weather",
				State: "output-available", Input: json.RawMessage(`{"city":"Hanoi"}`),
				Output: `{"tempC":31}`,
			},
			{Type: EnvelopePartTypeText, Text: "It is 31 degrees."},
		}},
		{Role: "user", Parts: []EnvelopePartUnion{{Type: EnvelopePartTypeText, Text: "and tomorrow?"}}},
	}
}

func TestToAIMessages_DecodedHistoryIsValidForARun(t *testing.T) {
	for i, message := range ToAIMessages(completedToolTurn()) {
		if err := message.Validate(); err != nil {
			t.Fatalf("messages[%d] (role %q) is not a valid run input: %v", i, message.Role, err)
		}
	}
}

func TestToAIMessages_ToolResultLeavesTheAssistantTurn(t *testing.T) {
	messages := ToAIMessages(completedToolTurn())

	var assistant, toolRole *aikit.Message
	for i := range messages {
		switch messages[i].Role {
		case aikit.RoleAssistant:
			assistant = &messages[i]
		case aikit.RoleTool:
			toolRole = &messages[i]
		}
	}

	if assistant == nil {
		t.Fatal("no assistant message survived the split")
	}
	for _, part := range assistant.Content {
		if part.Type == aikit.ContentPartTypeToolResult {
			t.Error("assistant turn still carries a tool_result")
		}
	}
	if !hasPart(assistant.Content, aikit.ContentPartTypeToolCall) {
		t.Error("assistant turn lost its tool_call")
	}
	if !hasPart(assistant.Content, aikit.ContentPartTypeText) {
		t.Error("assistant turn lost its text")
	}

	if toolRole == nil {
		t.Fatal("tool_result did not move to a tool-role message")
	}
	if got := toolRole.Content[0].ToolResultID; got != "call_1" {
		t.Errorf("tool result ID = %q, want call_1", got)
	}
}

// A user turn may legitimately carry a tool result; only assistant turns split.
func TestToAIMessages_UserToolResultIsNotSplit(t *testing.T) {
	messages := ToAIMessages([]EnvelopeMessage{{
		Role: "user", Parts: []EnvelopePartUnion{{
			Type: "tool-get_weather", ToolCallID: "call_9", ToolName: "get_weather",
			State: "output-available", Input: json.RawMessage(`{}`), Output: `{}`,
		}},
	}})

	if len(messages) != 1 {
		t.Fatalf("got %d messages, want 1 (no split for a user turn)", len(messages))
	}
	if messages[0].Role != aikit.RoleUser {
		t.Errorf("role = %q, want user", messages[0].Role)
	}
}

func hasPart(parts []aikit.ContentPart, kind aikit.ContentPartType) bool {
	for _, part := range parts {
		if part.Type == kind {
			return true
		}
	}
	return false
}
