package ainode

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
		role := aikit.Role(m.Role)
		// A completed tool part decodes into a tool_call *and* a tool_result.
		// Only the call belongs to the assistant turn: tool_result is valid
		// content for the tool role (which is what the agent itself emits in
		// history) and for a user turn, but never for an assistant one, so
		// leaving them together makes the decoder produce a message its own
		// validator rejects — and every multi-turn run with a prior tool call
		// fails before it starts.
		own, results := splitToolResults(role, parts)
		if len(own) > 0 {
			result = append(result, aikit.Message{Role: role, Content: own})
		}
		if len(results) > 0 {
			result = append(result, aikit.Message{Role: aikit.RoleTool, Content: results})
		}
		responses := approvalResponseParts(m.Parts)
		if len(responses) > 0 {
			result = append(result, aikit.Message{Role: aikit.RoleUser, Content: responses})
		}
	}
	return result
}

// splitToolResults separates tool_result parts out of an assistant turn,
// preserving order within each group. Other roles keep their parts as-is: a
// user message may legitimately carry a tool result.
func splitToolResults(
	role aikit.Role,
	parts []aikit.ContentPart,
) (own, results []aikit.ContentPart) {
	if role != aikit.RoleAssistant {
		return parts, nil
	}
	for _, part := range parts {
		if part.Type == aikit.ContentPartTypeToolResult {
			results = append(results, part)
			continue
		}
		own = append(own, part)
	}
	return own, results
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
			ifn := filenameOf(p)
			switch {
			case p.FileID != "":
				out = append(out, filePart("", "image", nil, p.FileID, ifn))
			case len(p.Data) > 0:
				out = append(out, filePart("", p.MediaType, p.Data, "", ifn))
			default:
				out = append(out, filePart(p.URL, "image", nil, "", ifn))
			}
		case EnvelopePartTypeFile:
			// Filename is canonical (v7); Name is the legacy fallback.
			// One normalized value reaches the provider-facing file part.
			fn := filenameOf(p)
			switch {
			case p.FileID != "":
				out = append(out, filePart("", p.MediaType, nil, p.FileID, fn))
			case len(p.Data) > 0:
				out = append(out, filePart("", p.MediaType, p.Data, "", fn))
			default:
				out = append(out, filePart(p.URL, p.MediaType, nil, "", fn))
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

// filenameOf resolves the provider-facing filename for a file/image part.
// Canonical v7 Filename takes precedence; legacy Name is the fallback so
// older wire payloads that only carry "name" keep working.
func filenameOf(p EnvelopePartUnion) string {
	if p.Filename != "" {
		return p.Filename
	}
	return p.Name
}
