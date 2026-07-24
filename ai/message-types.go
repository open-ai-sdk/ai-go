// Package ai provides the public API surface for the ai-go SDK.
package ai

import "encoding/json"

// Role identifies the author of a message in a conversation.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ContentPartType identifies the kind of content in a message part.
type ContentPartType string

const (
	// ContentPartTypeText is a plain-text part.
	ContentPartTypeText ContentPartType = "text"
	// ContentPartTypeFile is a file referenced by URL or data URI.
	ContentPartTypeFile ContentPartType = "file"
	// ContentPartTypeToolCall is a model-issued tool call (assistant turn only).
	ContentPartTypeToolCall ContentPartType = "tool_call"
	// ContentPartTypeToolResult is the result of a tool execution (tool turn only).
	ContentPartTypeToolResult ContentPartType = "tool_result"
	// ContentPartTypeReasoning carries prior reasoning text for history replay
	// (e.g. Claude extended thinking). Used in assistant messages when replaying
	// multi-step conversations that included a reasoning block.
	ContentPartTypeReasoning ContentPartType = "reasoning"
)

// ContentPart is a single part of a message (text, image, file, tool call/result, or reasoning).
// Only the fields matching the active Type are populated; all others are zero.
type ContentPart struct {
	// Type identifies which fields below are populated.
	Type ContentPartType

	// Text is set when Type == ContentPartTypeText.
	Text string

	// FileURL is the URL or data URI when Type == ContentPartTypeFile.
	FileURL string
	// MediaType is either a full IANA media type ("image/png") or just the
	// top-level segment ("image") when the subtype is unknown. Providers narrow
	// it as their API requires.
	MediaType string

	// Data holds inline binary content for file parts.
	// Exactly one of FileURL, Data, or FileID should be set per part.
	Data []byte
	// FileID is a provider-specific file identifier (e.g. OpenAI "file-abc123").
	// Exactly one of FileURL, Data, or FileID should be set per part.
	FileID string
	// Filename is the original filename for file parts (optional, used with Data or FileID).
	Filename string

	// ToolCallID is the unique ID for the tool call when Type == ContentPartTypeToolCall.
	ToolCallID string
	// ToolCallName is the tool function name when Type == ContentPartTypeToolCall.
	ToolCallName string
	// ToolCallArgs is the JSON-encoded arguments when Type == ContentPartTypeToolCall.
	ToolCallArgs json.RawMessage
	// ThoughtSignature is the Gemini thought signature for multi-turn tool calls.
	ThoughtSignature string

	// ToolResultID matches the ToolCallID that this result answers (Type == ContentPartTypeToolResult).
	ToolResultID string
	// ToolResultName is the name of the tool that produced this result (Type == ContentPartTypeToolResult).
	ToolResultName string
	// ToolResultOutput is the string result from the tool execution (Type == ContentPartTypeToolResult).
	ToolResultOutput string

	// ReasoningText holds the reasoning / thinking text when Type == ContentPartTypeReasoning.
	ReasoningText string
}

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

// Message is a single turn in a conversation.
type Message struct {
	Role    Role
	Content []ContentPart
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
