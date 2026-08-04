package tool

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// Output is an immutable, ordered model-visible tool output. Use the
// constructors in this package to make the intended content type explicit.
// In particular, Text never attempts to interpret JSON-looking strings.
type Output struct {
	parts []aikit.ToolResultContent
}

// Text creates literal text output.
func Text(value string) Output {
	return Output{parts: []aikit.ToolResultContent{aikit.TextToolResultContent(value)}}
}

// JSON creates explicit structured JSON output.
func JSON(value json.RawMessage) (Output, error) {
	return Content(aikit.JSONToolResultContent(value))
}

// Image creates inline image output.
func Image(data []byte, mediaType string) Output {
	return Output{parts: []aikit.ToolResultContent{aikit.ImageToolResultContent(data, mediaType)}}
}

// Content creates ordered output from typed content parts.
func Content(parts ...aikit.ToolResultContent) (Output, error) {
	cloned := make([]aikit.ToolResultContent, len(parts))
	for i, part := range parts {
		if err := validateContent(part); err != nil {
			return Output{}, err
		}
		cloned[i] = part.Clone()
	}
	return Output{parts: cloned}, nil
}

// Parts returns independently owned ordered content.
func (o Output) Parts() []aikit.ToolResultContent {
	if o.parts == nil {
		return nil
	}
	parts := make([]aikit.ToolResultContent, len(o.parts))
	for i := range o.parts {
		parts[i] = o.parts[i].Clone()
	}
	return parts
}

// Clone returns an independently owned Output.
func (o Output) Clone() Output { return Output{parts: o.Parts()} }

// LegacyJSON returns the compatibility raw-JSON representation used by
// Invokable.Invoke. It preserves the old typed New behavior: strings are JSON
// strings and arbitrary values are JSON. Rich multi-part values are encoded as
// their typed content slice rather than being flattened to placeholder text.
func (o Output) LegacyJSON() json.RawMessage {
	if len(o.parts) == 0 {
		return json.RawMessage("null")
	}
	if len(o.parts) == 1 {
		part := o.parts[0]
		switch part.Type {
		case aikit.ToolResultContentTypeJSON:
			return append(json.RawMessage(nil), part.JSON...)
		case aikit.ToolResultContentTypeText:
			value, err := json.Marshal(part.Text)
			if err == nil {
				return value
			}
			return nil
		}
	}
	value, err := json.Marshal(o.parts)
	if err != nil {
		return nil
	}
	return value
}

// ModelText is the compatibility presentation used by providers that only
// accept a string tool-result field. It never exposes host metadata.
func (o Output) ModelText() string {
	if len(o.parts) == 0 {
		return ""
	}
	if len(o.parts) == 1 {
		part := o.parts[0]
		if part.Type == aikit.ToolResultContentTypeText {
			return part.Text
		}
		if part.Type == aikit.ToolResultContentTypeJSON {
			return string(part.JSON)
		}
	}
	allText := true
	text := make([]string, len(o.parts))
	for i, part := range o.parts {
		if part.Type != aikit.ToolResultContentTypeText {
			allText = false
			break
		}
		text[i] = part.Text
	}
	if allText {
		return strings.Join(text, "\n")
	}
	return string(o.LegacyJSON())
}

func validateContent(part aikit.ToolResultContent) error {
	switch part.Type {
	case aikit.ToolResultContentTypeText:
		return nil
	case aikit.ToolResultContentTypeJSON:
		if !json.Valid(part.JSON) {
			return fmt.Errorf("tool: invalid JSON output")
		}
		return nil
	case aikit.ToolResultContentTypeImage, aikit.ToolResultContentTypeFile:
		if part.MediaType == "" {
			return fmt.Errorf("tool: %s output requires a media type", part.Type)
		}
		return nil
	default:
		return fmt.Errorf("tool: unsupported output content type %q", part.Type)
	}
}
