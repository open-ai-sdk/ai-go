package aikit

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
	ContentPartTypeText       ContentPartType = "text"
	ContentPartTypeFile       ContentPartType = "file"
	ContentPartTypeToolCall   ContentPartType = "tool_call"
	ContentPartTypeToolResult ContentPartType = "tool_result"
	ContentPartTypeReasoning  ContentPartType = "reasoning"
)

// ContentPart is a single part of a message.
type ContentPart struct {
	Type ContentPartType

	Text string

	FileURL   string
	MediaType string
	Data      []byte
	FileID    string
	Filename  string

	ToolCallID       string
	ToolCallName     string
	ToolCallArgs     json.RawMessage
	ThoughtSignature string

	ToolResultID     string
	ToolResultName   string
	ToolResultOutput string

	ReasoningText string
}

// Message is a single turn in a conversation.
type Message struct {
	Role    Role
	Content []ContentPart
}
