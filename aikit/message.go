package aikit

import (
	"encoding/json"
	"errors"
	"fmt"
)

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
	ContentPartTypeImage      ContentPartType = "image"
	ContentPartTypeAudio      ContentPartType = "audio"
	ContentPartTypeDocument   ContentPartType = "document"
	ContentPartTypeVideo      ContentPartType = "video"
	ContentPartTypeToolCall   ContentPartType = "tool_call"
	ContentPartTypeToolResult ContentPartType = "tool_result"
	// ContentPartTypeToolApprovalResponse carries a human approval decision
	// back to the stateless agent runtime in conversation history.
	ContentPartTypeToolApprovalResponse ContentPartType = "tool_approval_response"
	ContentPartTypeReasoning            ContentPartType = "reasoning"
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

	ToolResultID      string
	ToolResultName    string
	ToolResultOutput  string
	ToolResultContent []ToolResultContent
	// ToolResultApprovalSignature authenticates a runtime-produced result for
	// an approval-gated call when the consumed approval response is omitted
	// from clean continuation history.
	ToolResultApprovalSignature string
	ToolResultApprovalApproved  bool

	ToolApprovalID        string
	ToolApprovalSignature string
	ToolApprovalApproved  bool
	ToolApprovalReason    string

	ReasoningText string
}

// Message is a single turn in a conversation.
type Message struct {
	// ID is the provider's assistant-message identifier. It is distinct from
	// tool-call and approval identifiers.
	ID      string
	Role    Role
	Content []ContentPart
}

var (
	// ErrInvalidMessage reports a role/content contract violation.
	ErrInvalidMessage = errors.New("invalid message")
	// ErrWrongMessageRole reports use of a role-specific accessor on another role.
	ErrWrongMessageRole = errors.New("wrong message role")
)

// TextPart constructs a text content part.
func TextPart(text string) ContentPart { return ContentPart{Type: ContentPartTypeText, Text: text} }

// ImageURLPart constructs an explicit image part backed by a URL.
func ImageURLPart(url string) ContentPart {
	return ContentPart{Type: ContentPartTypeImage, FileURL: url, MediaType: "image"}
}

// ImageDataPart constructs an explicit image part and copies data.
func ImageDataPart(data []byte, mediaType string) ContentPart {
	return ContentPart{Type: ContentPartTypeImage, Data: cloneBytes(data), MediaType: mediaType}
}

// ImageFileIDPart constructs an explicit image part backed by a provider file ID.
func ImageFileIDPart(fileID string) ContentPart {
	return ContentPart{Type: ContentPartTypeImage, FileID: fileID, MediaType: "image"}
}

// AudioURLPart constructs an explicit audio part backed by a URL.
func AudioURLPart(url string, mediaType ...string) ContentPart {
	return mediaURLPart(ContentPartTypeAudio, url, firstString(mediaType))
}

// AudioDataPart constructs an explicit audio part and copies data.
func AudioDataPart(data []byte, mediaType string) ContentPart {
	return mediaDataPart(ContentPartTypeAudio, data, mediaType, "")
}

// AudioFileIDPart constructs an explicit audio part backed by a provider file ID.
func AudioFileIDPart(fileID string, mediaType ...string) ContentPart {
	return mediaFileIDPart(ContentPartTypeAudio, fileID, firstString(mediaType))
}

// DocumentURLPart constructs an explicit document part backed by a URL.
func DocumentURLPart(url, mediaType string, filename ...string) ContentPart {
	part := mediaURLPart(ContentPartTypeDocument, url, mediaType)
	part.Filename = firstString(filename)
	return part
}

// DocumentDataPart constructs an explicit document part and copies data.
func DocumentDataPart(data []byte, mediaType string, filename ...string) ContentPart {
	return mediaDataPart(ContentPartTypeDocument, data, mediaType, firstString(filename))
}

// DocumentFileIDPart constructs an explicit document part backed by a provider file ID.
func DocumentFileIDPart(fileID, mediaType string, filename ...string) ContentPart {
	part := mediaFileIDPart(ContentPartTypeDocument, fileID, mediaType)
	part.Filename = firstString(filename)
	return part
}

// VideoURLPart constructs an explicit video part backed by a URL.
func VideoURLPart(url string, mediaType ...string) ContentPart {
	return mediaURLPart(ContentPartTypeVideo, url, firstString(mediaType))
}

// VideoDataPart constructs an explicit video part and copies data.
func VideoDataPart(data []byte, mediaType string) ContentPart {
	return mediaDataPart(ContentPartTypeVideo, data, mediaType, "")
}

// VideoFileIDPart constructs an explicit video part backed by a provider file ID.
func VideoFileIDPart(fileID string, mediaType ...string) ContentPart {
	return mediaFileIDPart(ContentPartTypeVideo, fileID, firstString(mediaType))
}

// FilePart constructs a legacy file part. New code should prefer an explicit
// image, audio, document, or video constructor.
func FilePart(url, mediaType string) ContentPart {
	return ContentPart{Type: ContentPartTypeFile, FileURL: url, MediaType: mediaType}
}

// FileDataPart constructs a legacy inline file part and copies data.
func FileDataPart(data []byte, mediaType, filename string) ContentPart {
	return ContentPart{Type: ContentPartTypeFile, Data: cloneBytes(data), MediaType: mediaType, Filename: filename}
}

// FileIDPart constructs a legacy provider-hosted file part.
func FileIDPart(fileID, mediaType string) ContentPart {
	return ContentPart{Type: ContentPartTypeFile, FileID: fileID, MediaType: mediaType}
}

// ReasoningPart constructs an assistant reasoning part.
func ReasoningPart(text string) ContentPart {
	return ContentPart{Type: ContentPartTypeReasoning, ReasoningText: text}
}

// ToolCallPart constructs an assistant tool-call part and copies args.
func ToolCallPart(id, name string, args json.RawMessage) ContentPart {
	return ContentPart{
		Type: ContentPartTypeToolCall, ToolCallID: id, ToolCallName: name,
		ToolCallArgs: cloneRawMessage(args),
	}
}

// ToolResultPart constructs a literal-text tool-result part.
func ToolResultPart(id, name, output string) ContentPart {
	return ContentPart{
		Type: ContentPartTypeToolResult, ToolResultID: id,
		ToolResultName: name, ToolResultOutput: output,
	}
}

// ToolApprovalResponsePart constructs a user approval response part.
func ToolApprovalResponsePart(id, signature string, approved bool, reason string) ContentPart {
	return ContentPart{
		Type: ContentPartTypeToolApprovalResponse, ToolApprovalID: id,
		ToolApprovalSignature: signature, ToolApprovalApproved: approved,
		ToolApprovalReason: reason,
	}
}

// UserMessage creates a user message with one text part.
func UserMessage(text string) Message {
	return Message{Role: RoleUser, Content: []ContentPart{TextPart(text)}}
}

// AssistantMessage creates an assistant message with one text part.
func AssistantMessage(text string) Message {
	return Message{Role: RoleAssistant, Content: []ContentPart{TextPart(text)}}
}

// SystemMessage creates a system message with one text part.
func SystemMessage(text string) Message {
	return Message{Role: RoleSystem, Content: []ContentPart{TextPart(text)}}
}

func mediaURLPart(kind ContentPartType, url, mediaType string) ContentPart {
	return ContentPart{Type: kind, FileURL: url, MediaType: mediaType}
}

func mediaDataPart(kind ContentPartType, data []byte, mediaType, filename string) ContentPart {
	return ContentPart{Type: kind, Data: cloneBytes(data), MediaType: mediaType, Filename: filename}
}

func mediaFileIDPart(kind ContentPartType, fileID, mediaType string) ContentPart {
	return ContentPart{Type: kind, FileID: fileID, MediaType: mediaType}
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// Clone returns an independently owned copy of the content part.
func (p ContentPart) Clone() ContentPart {
	p.Data = cloneBytes(p.Data)
	p.ToolCallArgs = cloneRawMessage(p.ToolCallArgs)
	if p.ToolResultContent != nil {
		p.ToolResultContent = make([]ToolResultContent, len(p.ToolResultContent))
		for i := range p.ToolResultContent {
			p.ToolResultContent[i] = p.ToolResultContent[i].Clone()
		}
	}
	return p
}

// RichToolResultPart constructs a tool-result message part with ordered typed
// content. Text is never reinterpreted as JSON.
func RichToolResultPart(id, name string, content ...ToolResultContent) ContentPart {
	part := ContentPart{Type: ContentPartTypeToolResult, ToolResultID: id, ToolResultName: name}
	if content != nil {
		part.ToolResultContent = make([]ToolResultContent, len(content))
		for i := range content {
			part.ToolResultContent[i] = content[i].Clone()
		}
	}
	return part
}

// CloneContentParts returns an independently owned ordered copy of parts.
func CloneContentParts(parts []ContentPart) []ContentPart {
	if parts == nil {
		return nil
	}
	cloned := make([]ContentPart, len(parts))
	for i := range parts {
		cloned[i] = parts[i].Clone()
	}
	return cloned
}

// Clone returns an independently owned copy of the message.
func (m Message) Clone() Message {
	m.Content = CloneContentParts(m.Content)
	return m
}

// Validate enforces the role-aware message/content contract.
func (m Message) Validate() error { return ValidateMessage(m) }

// ValidateMessage enforces the role-aware message/content contract.
func ValidateMessage(m Message) error {
	if len(m.Content) == 0 {
		return fmt.Errorf("%w: content must not be empty", ErrInvalidMessage)
	}
	if m.ID != "" && m.Role != RoleAssistant {
		return fmt.Errorf("%w: message ID is only valid for assistant messages", ErrInvalidMessage)
	}
	for i, part := range m.Content {
		if !contentAllowedForRole(m.Role, part.Type) {
			return fmt.Errorf(
				"%w: content part %d of type %q is not valid for role %q",
				ErrInvalidMessage,
				i,
				part.Type,
				m.Role,
			)
		}
		if err := validateContentPart(part); err != nil {
			return fmt.Errorf("%w: content part %d: %w", ErrInvalidMessage, i, err)
		}
	}
	return nil
}

func contentAllowedForRole(role Role, kind ContentPartType) bool {
	switch role {
	case RoleSystem:
		return kind == ContentPartTypeText
	case RoleUser:
		switch kind {
		case ContentPartTypeText, ContentPartTypeToolResult, ContentPartTypeFile,
			ContentPartTypeImage, ContentPartTypeAudio, ContentPartTypeDocument,
			ContentPartTypeVideo, ContentPartTypeToolApprovalResponse:
			return true
		}
	case RoleAssistant:
		switch kind {
		case ContentPartTypeText, ContentPartTypeToolCall, ContentPartTypeReasoning, ContentPartTypeImage:
			return true
		}
	case RoleTool:
		return kind == ContentPartTypeToolResult
	}
	return false
}

func validateContentPart(part ContentPart) error {
	switch part.Type {
	case ContentPartTypeFile, ContentPartTypeImage, ContentPartTypeAudio,
		ContentPartTypeDocument, ContentPartTypeVideo:
		sources := 0
		if part.FileURL != "" {
			sources++
		}
		if len(part.Data) != 0 {
			sources++
		}
		if part.FileID != "" {
			sources++
		}
		if sources != 1 {
			return fmt.Errorf("media content must have exactly one of URL, data, or file ID (got %d)", sources)
		}
	case ContentPartTypeToolCall:
		if part.ToolCallID == "" || part.ToolCallName == "" || len(part.ToolCallArgs) == 0 {
			return errors.New("tool call ID, name, and arguments are required")
		}
	case ContentPartTypeToolResult:
		if part.ToolResultID == "" || part.ToolResultName == "" {
			return errors.New("tool result call ID and name are required")
		}
	case ContentPartTypeToolApprovalResponse:
		if part.ToolApprovalID == "" {
			return errors.New("tool approval ID is required")
		}
	}
	return nil
}

// SystemContent returns copied ordered content when the message is system-authored.
func (m Message) SystemContent() ([]ContentPart, error) { return m.contentForRole(RoleSystem) }

// UserContent returns copied ordered content when the message is user-authored.
func (m Message) UserContent() ([]ContentPart, error) { return m.contentForRole(RoleUser) }

// AssistantContent returns copied ordered content when the message is assistant-authored.
func (m Message) AssistantContent() ([]ContentPart, error) { return m.contentForRole(RoleAssistant) }

// ToolContent returns copied ordered content when the message uses the legacy tool role.
func (m Message) ToolContent() ([]ContentPart, error) { return m.contentForRole(RoleTool) }

func (m Message) contentForRole(role Role) ([]ContentPart, error) {
	if m.Role != role {
		return nil, fmt.Errorf("%w: got %q, want %q", ErrWrongMessageRole, m.Role, role)
	}
	return CloneContentParts(m.Content), nil
}

func cloneBytes(value []byte) []byte { return append([]byte(nil), value...) }

func cloneRawMessage(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}
