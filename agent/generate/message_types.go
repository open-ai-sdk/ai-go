package generate

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
	ContentPartTypeImage                = aikit.ContentPartTypeImage
	ContentPartTypeAudio                = aikit.ContentPartTypeAudio
	ContentPartTypeDocument             = aikit.ContentPartTypeDocument
	ContentPartTypeVideo                = aikit.ContentPartTypeVideo
	ContentPartTypeToolCall             = aikit.ContentPartTypeToolCall
	ContentPartTypeToolResult           = aikit.ContentPartTypeToolResult
	ContentPartTypeToolApprovalResponse = aikit.ContentPartTypeToolApprovalResponse
	ContentPartTypeReasoning            = aikit.ContentPartTypeReasoning
)

// TextPart constructs a text ContentPart.
func TextPart(text string) ContentPart {
	return aikit.TextPart(text)
}

// ImageURLPart constructs an explicit image ContentPart at a URL or data URI.
func ImageURLPart(url string) ContentPart {
	return aikit.ImageURLPart(url)
}

// FilePart constructs a file ContentPart from a URL or data URI.
func FilePart(url, mediaType string) ContentPart {
	return aikit.FilePart(url, mediaType)
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
	return aikit.ImageDataPart(data, mediaType)
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
	return aikit.ImageFileIDPart(fileID)
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
	return aikit.FileDataPart(data, mediaType, filename)
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
	return aikit.FileIDPart(fileID, mediaType)
}

func AudioURLPart(url string, mediaType ...string) ContentPart {
	return aikit.AudioURLPart(url, mediaType...)
}

func AudioDataPart(data []byte, mediaType string) ContentPart {
	return aikit.AudioDataPart(data, mediaType)
}

func AudioFileIDPart(fileID string, mediaType ...string) ContentPart {
	return aikit.AudioFileIDPart(fileID, mediaType...)
}

func DocumentURLPart(url, mediaType string, filename ...string) ContentPart {
	return aikit.DocumentURLPart(url, mediaType, filename...)
}

func DocumentDataPart(data []byte, mediaType string, filename ...string) ContentPart {
	return aikit.DocumentDataPart(data, mediaType, filename...)
}

func DocumentFileIDPart(fileID, mediaType string, filename ...string) ContentPart {
	return aikit.DocumentFileIDPart(fileID, mediaType, filename...)
}

func VideoURLPart(url string, mediaType ...string) ContentPart {
	return aikit.VideoURLPart(url, mediaType...)
}

func VideoDataPart(data []byte, mediaType string) ContentPart {
	return aikit.VideoDataPart(data, mediaType)
}

func VideoFileIDPart(fileID string, mediaType ...string) ContentPart {
	return aikit.VideoFileIDPart(fileID, mediaType...)
}

func TextToolResultContent(text string) ToolResultContent {
	return aikit.TextToolResultContent(text)
}

func JSONToolResultContent(raw json.RawMessage) ToolResultContent {
	return aikit.JSONToolResultContent(raw)
}

func ImageToolResultContent(data []byte, mediaType string) ToolResultContent {
	return aikit.ImageToolResultContent(data, mediaType)
}

func ParseToolResultJSON(output string) (ToolResultContent, error) {
	return aikit.ParseToolResultJSON(output)
}

// ReasoningPart constructs a reasoning ContentPart for history replay.
// Use this when reconstructing assistant messages that included a reasoning block
// (e.g. Claude extended thinking) so that the model can continue from prior reasoning.
func ReasoningPart(reasoningText string) ContentPart {
	return aikit.ReasoningPart(reasoningText)
}

// ToolCallPart constructs a tool-call ContentPart for assistant messages.
func ToolCallPart(id, name string, args json.RawMessage) ContentPart {
	return aikit.ToolCallPart(id, name, args)
}

// ToolResultPart constructs a tool-result ContentPart for tool messages.
func ToolResultPart(id, name, output string) ContentPart {
	return aikit.ToolResultPart(id, name, output)
}

func RichToolResultPart(id, name string, content ...ToolResultContent) ContentPart {
	return aikit.RichToolResultPart(id, name, content...)
}

// ToolApprovalResponsePart carries a decision for a previously emitted tool
// approval request. Add it as the only content of a user message in the next
// request's history and echo the request signature. The agent verifies the
// signature and resolves the matching pending tool call without retaining
// server state.
func ToolApprovalResponsePart(approvalID, signature string, approved bool, reason string) ContentPart {
	return aikit.ToolApprovalResponsePart(approvalID, signature, approved, reason)
}

// UserMessage creates a user message with a single text part.
func UserMessage(text string) Message {
	return aikit.UserMessage(text)
}

// AssistantMessage creates an assistant message with a single text part.
func AssistantMessage(text string) Message {
	return aikit.AssistantMessage(text)
}

// SystemMessage creates a system message with a single text part.
func SystemMessage(text string) Message {
	return aikit.SystemMessage(text)
}
