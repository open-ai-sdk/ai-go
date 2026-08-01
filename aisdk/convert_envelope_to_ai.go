package aisdk

import (
	"encoding/json"
	"strings"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// ToAIMessages converts a slice of EnvelopeMessage to ai.Message values.
// If a message has Parts, they are converted via ToAIContentParts.
// Otherwise, Content is used as a single text part.
func ToAIMessages(msgs []EnvelopeMessage) []aikit.Message {
	result := make([]aikit.Message, 0, len(msgs))
	for _, m := range msgs {
		var parts []aikit.ContentPart
		if len(m.Parts) > 0 {
			parts = ToAIContentParts(m.Parts)
		} else {
			parts = []aikit.ContentPart{{Type: aikit.ContentPartTypeText, Text: m.Content}}
		}
		if len(parts) > 0 {
			result = append(result, aikit.Message{
				Role:    aikit.Role(m.Role),
				Content: parts,
			})
		}
		responses := approvalResponseParts(m.Parts)
		if len(responses) > 0 {
			result = append(result, aikit.Message{Role: aikit.RoleUser, Content: responses})
		}
	}
	return result
}

// ToAIContentParts converts a slice of EnvelopePartUnion to ai.ContentPart values.
// Priority for image/file parts: FileID > Data > URL.
func ToAIContentParts(parts []EnvelopePartUnion) []aikit.ContentPart {
	out := make([]aikit.ContentPart, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case EnvelopePartTypeText:
			out = append(out, aikit.ContentPart{Type: aikit.ContentPartTypeText, Text: p.Text})
		case EnvelopePartTypeImage:
			switch {
			case p.FileID != "":
				out = append(out, filePart("", "image", nil, p.FileID, ""))
			case len(p.Data) > 0:
				out = append(out, filePart("", p.MediaType, p.Data, "", ""))
			default:
				out = append(out, filePart(p.URL, "image", nil, "", ""))
			}
		case EnvelopePartTypeFile:
			switch {
			case p.FileID != "":
				out = append(out, filePart("", p.MediaType, nil, p.FileID, p.Name))
			case len(p.Data) > 0:
				out = append(out, filePart("", p.MediaType, p.Data, "", p.Name))
			default:
				out = append(out, filePart(p.URL, p.MediaType, nil, "", p.Name))
			}
		case EnvelopePartTypeToolInvocation, EnvelopePartTypeDynamicTool:
			out = append(out, toolInvocationParts(p)...)
		default:
			if strings.HasPrefix(string(p.Type), "tool-") {
				out = append(out, toolInvocationParts(p)...)
			}
		}
	}
	return out
}

// toolInvocationParts converts a tool-invocation EnvelopePartUnion into the
// corresponding ai.ContentPart(s). Only finalized states ("call", "result")
// produce parts; partial/unknown states are silently skipped.
//
// For state "result", both a ToolCallPart (for assistant context) and a
// ToolResultPart (for tool-role context) are emitted. Callers that need role
// separation should filter by ContentPartType after conversion.
func toolInvocationParts(p EnvelopePartUnion) []aikit.ContentPart {
	toolName := envelopeToolName(p)
	call := aikit.ContentPart{
		Type: aikit.ContentPartTypeToolCall, ToolCallID: p.ToolCallID,
		ToolCallName: toolName, ToolCallArgs: p.Input,
	}
	if p.Approval != nil {
		call.ToolApprovalID = p.Approval.ID
		call.ToolApprovalSignature = p.Approval.Signature
	}
	switch p.State {
	case "call", "partial-call", "input-streaming", "input-available",
		"approval-requested", "approval-responded":
		return []aikit.ContentPart{call}
	case "result", "output-available", "output-denied":
		return []aikit.ContentPart{
			call,
			{
				Type: aikit.ContentPartTypeToolResult, ToolResultID: p.ToolCallID,
				ToolResultName: toolName, ToolResultOutput: envelopeToolOutput(p.Output),
			},
		}
	case "output-error":
		return []aikit.ContentPart{
			call,
			{
				Type: aikit.ContentPartTypeToolResult, ToolResultID: p.ToolCallID,
				ToolResultName: toolName, ToolResultOutput: p.ErrorText,
			},
		}
	default:
		// "error" and unknown states: skip gracefully.
		return nil
	}
}

func envelopeToolOutput(output any) string {
	if text, ok := output.(string); ok {
		return text
	}
	if output == nil {
		return ""
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func envelopeToolName(part EnvelopePartUnion) string {
	if part.Type == EnvelopePartTypeDynamicTool || part.Type == EnvelopePartTypeToolInvocation {
		return part.ToolName
	}
	return strings.TrimPrefix(string(part.Type), "tool-")
}

func approvalResponseParts(parts []EnvelopePartUnion) []aikit.ContentPart {
	responses := make([]aikit.ContentPart, 0)
	for _, part := range parts {
		approval := part.Approval
		if part.State != "approval-responded" || approval == nil || approval.Approved == nil {
			continue
		}
		responses = append(responses, aikit.ContentPart{
			Type:           aikit.ContentPartTypeToolApprovalResponse,
			ToolApprovalID: approval.ID, ToolApprovalSignature: approval.Signature,
			ToolApprovalApproved: *approval.Approved, ToolApprovalReason: approval.Reason,
		})
	}
	return responses
}

func filePart(url, mediaType string, data []byte, fileID, filename string) aikit.ContentPart {
	return aikit.ContentPart{
		Type: aikit.ContentPartTypeFile, FileURL: url, MediaType: mediaType,
		Data: data, FileID: fileID, Filename: filename,
	}
}
