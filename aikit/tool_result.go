package aikit

import (
	"encoding/json"
	"errors"

	"github.com/open-ai-sdk/ai-go/internal/jsonclone"
)

const (
	ToolResultContentTypeText  = "text"
	ToolResultContentTypeFile  = "file"
	ToolResultContentTypeJSON  = "json"
	ToolResultContentTypeImage = "image"
)

// ToolResultDisposition is the single outcome classification for a call.
type ToolResultDisposition string

const (
	ToolResultSuccess ToolResultDisposition = "success"
	ToolResultError   ToolResultDisposition = "error"
	ToolResultDenied  ToolResultDisposition = "denied"
	ToolResultRefused ToolResultDisposition = "refused"
	ToolResultSkipped ToolResultDisposition = "skipped"
)

// Valid reports whether the disposition is one of the defined outcomes.
func (d ToolResultDisposition) Valid() bool {
	switch d {
	case ToolResultSuccess, ToolResultError, ToolResultDenied, ToolResultRefused, ToolResultSkipped:
		return true
	default:
		return false
	}
}

// ToolResultContent represents a single content part in a tool result.
type ToolResultContent struct {
	Type      string
	Text      string
	JSON      json.RawMessage
	Data      []byte
	MediaType string
}

// TextToolResultContent constructs literal text. JSON-looking text remains text.
func TextToolResultContent(text string) ToolResultContent {
	return ToolResultContent{Type: ToolResultContentTypeText, Text: text}
}

// JSONToolResultContent constructs structured JSON and copies raw.
func JSONToolResultContent(raw json.RawMessage) ToolResultContent {
	return ToolResultContent{Type: ToolResultContentTypeJSON, JSON: cloneRawMessage(raw)}
}

// ImageToolResultContent constructs inline image content and copies data.
func ImageToolResultContent(data []byte, mediaType string) ToolResultContent {
	return ToolResultContent{Type: ToolResultContentTypeImage, Data: cloneBytes(data), MediaType: mediaType}
}

// ToolResultTextContent is an alternate noun-first spelling.
func ToolResultTextContent(text string) ToolResultContent { return TextToolResultContent(text) }

// ToolResultJSONContent is an alternate noun-first spelling.
func ToolResultJSONContent(raw json.RawMessage) ToolResultContent { return JSONToolResultContent(raw) }

// ToolResultImageContent is an alternate noun-first spelling.
func ToolResultImageContent(data []byte, mediaType string) ToolResultContent {
	return ImageToolResultContent(data, mediaType)
}

// ParseToolResultJSON explicitly parses model-visible output as structured JSON.
func ParseToolResultJSON(output string) (ToolResultContent, error) {
	raw := json.RawMessage(output)
	if !json.Valid(raw) {
		return ToolResultContent{}, errors.New("invalid JSON tool result")
	}
	return JSONToolResultContent(raw), nil
}

// Clone returns an independently owned copy.
func (c ToolResultContent) Clone() ToolResultContent {
	c.JSON = cloneRawMessage(c.JSON)
	c.Data = cloneBytes(c.Data)
	return c
}

// ToolResult holds the output of a single tool invocation.
type ToolResult struct {
	ID     string
	Name   string
	Args   string
	Output string
	// ModelOutput is the already-computed provider-history representation.
	// ModelOutputSet distinguishes an intentional empty transform result.
	ModelOutput    string
	ModelOutputSet bool
	// ApprovalSignature authenticates model-visible output produced while
	// resolving a signed history approval.
	ApprovalSignature        string
	ApprovalApproved         bool
	ApprovalID               string
	ApprovalRequestSignature string
	// Error retains typed tool failure classification for agent and wire
	// adapters. Output remains the model-visible raw string.
	Error       error `json:"-"`
	Content     []ToolResultContent
	Disposition ToolResultDisposition
	// Metadata is host-only metadata. It is intentionally excluded from JSON
	// and provider history serialization.
	Metadata map[string]any `json:"-"`
}

// Clone returns an independently owned copy of the tool result's structured content.
func (r ToolResult) Clone() ToolResult {
	if r.Content != nil {
		content := r.Content
		r.Content = make([]ToolResultContent, len(r.Content))
		for i := range content {
			r.Content[i] = content[i].Clone()
		}
	}
	r.Metadata = jsonclone.Map(r.Metadata)
	return r
}
