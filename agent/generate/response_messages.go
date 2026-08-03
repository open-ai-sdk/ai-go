package generate

import (
	"encoding/json"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// Response contains messages callers use to continue multi-step conversations.
type Response struct {
	Messages []Message
}

// ResponseMessagesForStep converts a completed step into continuation messages.
func ResponseMessagesForStep(step StepOutput, tools *ToolSet) []Message {
	var messages []Message

	assistantParts := cloneContentParts(step.Content)
	if len(assistantParts) == 0 {
		assistantParts = make([]ContentPart, 0, len(step.ToolCalls)+2)
	}
	if step.Reasoning != "" && !containsContentType(assistantParts, ContentPartTypeReasoning) {
		assistantParts = append(assistantParts, ReasoningPart(step.Reasoning))
	}
	if step.Text != "" && !containsContentType(assistantParts, ContentPartTypeText) {
		assistantParts = append(assistantParts, TextPart(step.Text))
	}
	for _, tc := range step.ToolCalls {
		part := ContentPart{
			Type: ContentPartTypeToolCall, ToolCallID: tc.ID, ToolCallName: tc.Name,
			ToolCallArgs: append(json.RawMessage(nil), tc.Args...), ThoughtSignature: tc.ThoughtSignature,
			ToolApprovalID: tc.ApprovalID, ToolApprovalSignature: tc.ApprovalSignature,
		}
		if index := toolCallContentIndex(assistantParts, tc.ID); index >= 0 {
			assistantParts[index] = part
		} else {
			assistantParts = append(assistantParts, part)
		}
	}
	if len(assistantParts) > 0 {
		messages = append(messages, Message{
			ID:      step.MessageID,
			Role:    RoleAssistant,
			Content: assistantParts,
		})
	}

	for _, tr := range step.ToolResults {
		messages = append(messages, responseMessageForToolResult(tr, tools))
	}

	return messages
}

func responseMessageForToolResult(result ToolResult, tools *ToolSet) Message {
	part := ToolResultPart(result.ID, result.Name, responseMessageToolOutput(result, tools))
	if result.Content != nil {
		part.ToolResultContent = make([]ToolResultContent, len(result.Content))
		for i := range result.Content {
			part.ToolResultContent[i] = result.Content[i].Clone()
		}
	}
	part.ToolApprovalID = result.ApprovalID
	part.ToolResultApprovalSignature = result.ApprovalSignature
	part.ToolResultApprovalApproved = result.ApprovalApproved
	return Message{Role: RoleTool, Content: []ContentPart{part}}
}

func containsContentType(parts []ContentPart, contentType ContentPartType) bool {
	for _, part := range parts {
		if part.Type == contentType {
			return true
		}
	}
	return false
}

func toolCallContentIndex(parts []ContentPart, id string) int {
	for i, part := range parts {
		if part.Type == ContentPartTypeToolCall && part.ToolCallID == id {
			return i
		}
	}
	return -1
}

func cloneContentParts(parts []ContentPart) []ContentPart {
	return aikit.CloneContentParts(parts)
}

func cloneMessages(messages []Message) []Message {
	if messages == nil {
		return nil
	}
	cloned := make([]Message, len(messages))
	for i := range messages {
		cloned[i] = messages[i].Clone()
	}
	return cloned
}

func transcriptMessages(initial, generated []Message) []Message {
	if initial == nil && generated == nil {
		return nil
	}
	transcript := make([]Message, 0, len(initial)+len(generated))
	transcript = append(transcript, cloneMessages(initial)...)
	transcript = append(transcript, cloneMessages(generated)...)
	return transcript
}

// ResponseMessagesForSteps converts all completed steps into continuation
// messages in execution order.
func ResponseMessagesForSteps(steps []StepOutput, tools *ToolSet) []Message {
	if len(steps) == 0 {
		return nil
	}
	var messages []Message
	for _, step := range steps {
		messages = append(messages, ResponseMessagesForStep(step, tools)...)
	}
	return messages
}

func responseMessageToolOutput(result ToolResult, tools *ToolSet) string {
	if result.ModelOutputSet {
		return result.ModelOutput
	}
	if tools == nil {
		return result.Output
	}
	for _, def := range tools.DefinitionsSnapshot() {
		if def.Name == result.Name {
			if def.ToModelOutput != nil {
				return def.ToModelOutput(result.Output)
			}
			break
		}
	}
	return result.Output
}
