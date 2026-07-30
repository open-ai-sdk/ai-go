package einoadapter

import (
	"encoding/json"

	"github.com/cloudwego/eino/schema"

	"github.com/open-ai-sdk/ai-go/aisdk"
)

// ToAgenticMessages converts a validated v7 history into the model request.
//
// Ported from ui/convert-to-model-messages.ts, with one structural change forced by Eino:
// there is no `tool` role. AgenticRoleType is system|user|assistant
// (schema/agentic_message.go:64-69), and putting a FunctionToolResult under assistant role
// hard-errors inside the provider — agenticclaude/convertor.go:217-230 accepts only
// AssistantGenText | Reasoning | FunctionToolCall | ServerToolCall | ServerToolResult and
// returns "invalid content block type %q with assistant role" for anything else.
//
// Eino's own tools node shows the intended shape: tool results are USER-role messages,
// one per result (compose/agentic_tools_node.go:101-108). They are deliberately not
// pre-merged, because agenticclaude's mergeAdjacentToolResults (:88-105) does that itself.
//
// The conversion is lossy by design, and the loss is not a bug to fix here: source-url,
// source-document, and data-* have no Eino block, and mcp_* / assistant_gen_image are
// inbound-rejected by the provider under every role. What the MODEL needs and what a UI
// needs to re-render are different things — PersistedMessageBuilder is the round-trip
// path, not this one.
func ToAgenticMessages(messages []aisdk.UIMessage, opts ...ConvertOption) ([]*schema.AgenticMessage, error) {
	cfg := convertConfig{}
	for _, o := range opts {
		o(&cfg)
	}

	var out []*schema.AgenticMessage
	for i := range messages {
		m := &messages[i]
		switch m.Role {
		case aisdk.UIRoleSystem:
			if blocks := systemBlocks(m); len(blocks) > 0 {
				out = append(out, &schema.AgenticMessage{
					Role: schema.AgenticRoleTypeSystem, ContentBlocks: blocks,
				})
			}
		case aisdk.UIRoleUser:
			if blocks := userBlocks(m); len(blocks) > 0 {
				out = append(out, &schema.AgenticMessage{
					Role: schema.AgenticRoleTypeUser, ContentBlocks: blocks,
				})
			}
		case aisdk.UIRoleAssistant:
			msgs, err := assistantMessages(m, cfg)
			if err != nil {
				return nil, err
			}
			out = append(out, msgs...)
		}
	}
	return out, nil
}

// ConvertOption configures the conversion.
type ConvertOption func(*convertConfig)

type convertConfig struct {
	// convertDataPart lets an application map its own data-* parts into blocks. Without
	// it they are dropped, matching the reference.
	convertDataPart func(name string, data json.RawMessage) *schema.ContentBlock
}

// WithDataPartConverter supplies a mapping for data-* parts.
func WithDataPartConverter(fn func(name string, data json.RawMessage) *schema.ContentBlock) ConvertOption {
	return func(c *convertConfig) { c.convertDataPart = fn }
}

func systemBlocks(m *aisdk.UIMessage) []*schema.ContentBlock {
	var blocks []*schema.ContentBlock
	for i := range m.Parts {
		if m.Parts[i].Type == aisdk.UIPartText && m.Parts[i].Text != "" {
			blocks = append(blocks, schema.NewContentBlock(
				&schema.UserInputText{Text: m.Parts[i].Text}))
		}
	}
	return blocks
}

func userBlocks(m *aisdk.UIMessage) []*schema.ContentBlock {
	var blocks []*schema.ContentBlock
	for i := range m.Parts {
		p := &m.Parts[i]
		switch {
		case p.Type == aisdk.UIPartText && p.Text != "":
			blocks = append(blocks, schema.NewContentBlock(
				&schema.UserInputText{Text: p.Text}))

		case p.Type == aisdk.UIPartFile:
			blocks = append(blocks, userFileBlock(p))
		}
		// source-*, data-*, and tool parts on a user message are dropped: the first two
		// have no Eino block, and a tool part is not something a user authors.
	}
	return blocks
}

// userFileBlock routes a file to the image block when its media type is an image, since
// providers treat images differently from generic attachments.
func userFileBlock(p *aisdk.UIMessagePart) *schema.ContentBlock {
	if isImageMediaType(p.MediaType) {
		return schema.NewContentBlock(&schema.UserInputImage{
			URL: p.URL, MIMEType: p.MediaType,
		})
	}
	return schema.NewContentBlock(&schema.UserInputFile{
		URL: p.URL, MIMEType: p.MediaType, Name: p.Filename,
	})
}

func isImageMediaType(mt string) bool {
	return len(mt) >= 6 && mt[:6] == "image/"
}
