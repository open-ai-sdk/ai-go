package ai

import (
	"encoding/json"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

func TextPart(text string) ContentPart           { return aikit.TextPart(text) }
func ImageURLPart(url string) ContentPart        { return aikit.ImageURLPart(url) }
func FilePart(url, mediaType string) ContentPart { return aikit.FilePart(url, mediaType) }
func ImageDataPart(data []byte, mediaType string) ContentPart {
	return aikit.ImageDataPart(data, mediaType)
}
func ImageFileIDPart(fileID string) ContentPart { return aikit.ImageFileIDPart(fileID) }
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

func FileDataPart(data []byte, mediaType, filename string) ContentPart {
	return aikit.FileDataPart(data, mediaType, filename)
}
func FileIDPart(fileID, mediaType string) ContentPart { return aikit.FileIDPart(fileID, mediaType) }
func ReasoningPart(text string) ContentPart           { return aikit.ReasoningPart(text) }
func ToolCallPart(id, name string, args json.RawMessage) ContentPart {
	return aikit.ToolCallPart(id, name, args)
}

func ToolResultPart(id, name, output string) ContentPart {
	return aikit.ToolResultPart(id, name, output)
}

func RichToolResultPart(id, name string, content ...ToolResultContent) ContentPart {
	return aikit.RichToolResultPart(id, name, content...)
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

func ToolApprovalResponsePart(id, signature string, approved bool, reason string) ContentPart {
	return aikit.ToolApprovalResponsePart(id, signature, approved, reason)
}
func UserMessage(text string) Message      { return aikit.UserMessage(text) }
func AssistantMessage(text string) Message { return aikit.AssistantMessage(text) }
func SystemMessage(text string) Message    { return aikit.SystemMessage(text) }

func OutputText() *OutputSchema { return &llm.OutputSchema{Type: "text"} }
func OutputJSONObject() *OutputSchema {
	return &llm.OutputSchema{Type: "json_object"}
}

func OutputObject(schema map[string]any) *OutputSchema {
	return &llm.OutputSchema{Type: "object", Schema: schema}
}

func OutputArray(itemSchema map[string]any) *OutputSchema {
	return &llm.OutputSchema{
		Type: "array",
		Schema: map[string]any{
			"type":  "array",
			"items": itemSchema,
		},
	}
}

func ToolChoiceSpecific(name string) ToolChoice {
	return aikit.ToolChoice{Type: "tool", ToolName: name}
}
