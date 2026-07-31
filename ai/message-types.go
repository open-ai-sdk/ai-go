// Package ai provides the public API surface for the ai-go SDK.
package ai

import (
	"encoding/json"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// Message vocabulary is owned by the dependency-free aikit package. These
// aliases preserve the established ai package surface.
type (
	Role            = aikit.Role
	ContentPartType = aikit.ContentPartType
	ContentPart     = aikit.ContentPart
	Message         = aikit.Message
)

const (
	RoleSystem    = aikit.RoleSystem
	RoleUser      = aikit.RoleUser
	RoleAssistant = aikit.RoleAssistant
	RoleTool      = aikit.RoleTool

	ContentPartTypeText                 = aikit.ContentPartTypeText
	ContentPartTypeFile                 = aikit.ContentPartTypeFile
	ContentPartTypeToolCall             = aikit.ContentPartTypeToolCall
	ContentPartTypeToolResult           = aikit.ContentPartTypeToolResult
	ContentPartTypeToolApprovalResponse = aikit.ContentPartTypeToolApprovalResponse
	ContentPartTypeReasoning            = aikit.ContentPartTypeReasoning
)

// TextPart constructs a text ContentPart.
func TextPart(text string) ContentPart {
	return ContentPart{Type: ContentPartTypeText, Text: text}
}

// ImageURLPart constructs a file ContentPart for an image at a URL or data URI.
// Images have no dedicated part kind — this is a convenience over FilePart.
// MediaType is set to the bare top-level segment "image" because the subtype is
// unknown here; without it, encoders that route on media type would treat the
// part as a generic file and drop the image.
func ImageURLPart(url string) ContentPart {
	return ContentPart{Type: ContentPartTypeFile, FileURL: url, MediaType: "image"}
}

// FilePart constructs a file ContentPart from a URL or data URI.
func FilePart(url, mediaType string) ContentPart {
	return ContentPart{Type: ContentPartTypeFile, FileURL: url, MediaType: mediaType}
}

// ImageDataPart constructs an image ContentPart from inline binary data.
// Use this when you have raw image bytes in memory (e.g. read from disk or
// received over the network) and want to send the image inline to the model.
// The mediaType must be an image media type such as "image/png" or "image/jpeg",
// or the bare top-level segment "image" when the subtype is unknown.
//
// Example:
//
//	data, _ := os.ReadFile("screenshot.png")
//	part := ai.ImageDataPart(data, "image/png")
func ImageDataPart(data []byte, mediaType string) ContentPart {
	return ContentPart{Type: ContentPartTypeFile, Data: data, MediaType: mediaType}
}

// ImageFileIDPart constructs an image ContentPart referencing a provider-hosted file.
// Use this when the image has already been uploaded to the provider (e.g. via the
// OpenAI Files API) and you have a file ID such as "file-abc123". Sending a file ID
// avoids re-uploading the binary on every request.
//
// Example:
//
//	part := ai.ImageFileIDPart("file-abc123")
func ImageFileIDPart(fileID string) ContentPart {
	return ContentPart{Type: ContentPartTypeFile, FileID: fileID, MediaType: "image"}
}

// FileDataPart constructs a file ContentPart from inline binary data.
// Use this when you have raw file bytes in memory and want to send the file
// inline to the model (e.g. a PDF document for summarisation). The filename
// parameter is forwarded to the provider and may appear in citations or logs.
//
// Example:
//
//	data, _ := os.ReadFile("report.pdf")
//	part := ai.FileDataPart(data, "application/pdf", "report.pdf")
func FileDataPart(data []byte, mediaType, filename string) ContentPart {
	return ContentPart{Type: ContentPartTypeFile, Data: data, MediaType: mediaType, Filename: filename}
}

// FileIDPart constructs a file ContentPart referencing a provider-hosted file.
// Use this when a non-image file has already been uploaded to the provider
// (e.g. via the OpenAI Files API) and you have a file ID such as "file-xyz".
// The mediaType hints to the provider how the file should be interpreted.
//
// Example:
//
//	part := ai.FileIDPart("file-xyz", "application/pdf")
func FileIDPart(fileID, mediaType string) ContentPart {
	return ContentPart{Type: ContentPartTypeFile, FileID: fileID, MediaType: mediaType}
}

// ReasoningPart constructs a reasoning ContentPart for history replay.
// Use this when reconstructing assistant messages that included a reasoning block
// (e.g. Claude extended thinking) so that the model can continue from prior reasoning.
func ReasoningPart(reasoningText string) ContentPart {
	return ContentPart{Type: ContentPartTypeReasoning, ReasoningText: reasoningText}
}

// ToolCallPart constructs a tool-call ContentPart for assistant messages.
func ToolCallPart(id, name string, args json.RawMessage) ContentPart {
	return ContentPart{
		Type:         ContentPartTypeToolCall,
		ToolCallID:   id,
		ToolCallName: name,
		ToolCallArgs: args,
	}
}

// ToolResultPart constructs a tool-result ContentPart for tool messages.
func ToolResultPart(id, name, output string) ContentPart {
	return ContentPart{
		Type:             ContentPartTypeToolResult,
		ToolResultID:     id,
		ToolResultName:   name,
		ToolResultOutput: output,
	}
}

// ToolApprovalResponsePart carries a decision for a previously emitted tool
// approval request. Add it to the next request's message history; the agent
// resolves the matching pending tool call without retaining server state.
func ToolApprovalResponsePart(approvalID string, approved bool, reason string) ContentPart {
	return ContentPart{
		Type:                 ContentPartTypeToolApprovalResponse,
		ToolApprovalID:       approvalID,
		ToolApprovalApproved: approved,
		ToolApprovalReason:   reason,
	}
}

// UserMessage creates a user message with a single text part.
func UserMessage(text string) Message {
	return Message{Role: RoleUser, Content: []ContentPart{TextPart(text)}}
}

// AssistantMessage creates an assistant message with a single text part.
func AssistantMessage(text string) Message {
	return Message{Role: RoleAssistant, Content: []ContentPart{TextPart(text)}}
}

// SystemMessage creates a system message with a single text part.
func SystemMessage(text string) Message {
	return Message{Role: RoleSystem, Content: []ContentPart{TextPart(text)}}
}
