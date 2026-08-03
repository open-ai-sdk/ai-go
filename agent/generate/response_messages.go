package generate

import "encoding/json"

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
		part := ToolResultPart(tr.ID, tr.Name, responseMessageToolOutput(tr, tools))
		if len(tr.Content) > 0 {
			part.ToolResultContent = make([]ToolResultContent, len(tr.Content))
			for i := range tr.Content {
				part.ToolResultContent[i] = tr.Content[i].Clone()
			}
		}
		part.ToolApprovalID = tr.ApprovalID
		part.ToolResultApprovalSignature = tr.ApprovalSignature
		part.ToolResultApprovalApproved = tr.ApprovalApproved
		messages = append(messages, Message{
			Role:    RoleTool,
			Content: []ContentPart{part},
		})
	}

	return messages
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
	if parts == nil {
		return nil
	}
	cloned := make([]ContentPart, len(parts))
	for i, part := range parts {
		cloned[i] = part.Clone()
	}
	return cloned
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
	for _, def := range tools.Definitions {
		if def.Name == result.Name {
			if def.ToModelOutput != nil {
				return def.ToModelOutput(result.Output)
			}
			break
		}
	}
	return result.Output
}
