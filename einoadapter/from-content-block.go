package einoadapter

import (
	"encoding/json"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/open-ai-sdk/ai-go/aisdk"
)

// convertBlock maps one ContentBlock, opening or closing blocks in ConvState as needed.
// Returns false when the consumer stopped iterating.
func convertBlock(
	b *schema.ContentBlock, index int, st *ConvState,
	yield func(aisdk.StepEvent, error) bool,
) bool {
	switch b.Type {
	case schema.ContentBlockTypeAssistantGenText:
		return convertText(b, index, st, yield)

	case schema.ContentBlockTypeReasoning:
		return convertReasoning(b, index, st, yield)

	case schema.ContentBlockTypeFunctionToolCall:
		return convertFunctionToolCall(b, index, st, yield)

	case schema.ContentBlockTypeFunctionToolResult:
		return convertFunctionToolResult(b, st, yield)

	case schema.ContentBlockTypeServerToolCall:
		return convertServerToolCall(b, index, st, yield)

	case schema.ContentBlockTypeServerToolResult:
		return convertServerToolResult(b, index, st, yield)

	case schema.ContentBlockTypeToolSearchResult:
		return convertToolSearchResult(b, yield)

	default:
		// Deliberately dropped, each for a stated reason:
		//
		//   mcp_tool_call / mcp_tool_result / mcp_list_tools_result /
		//   mcp_tool_approval_request / mcp_tool_approval_response
		//   assistant_gen_image / assistant_gen_audio / assistant_gen_video
		//     agenticclaude rejects all of these on INBOUND, under both assistant and
		//     user role. Emitting them would put content on the wire that cannot survive
		//     the next turn's history round-trip — the client would send it back and the
		//     provider would reject the request. Re-add when a provider handles them
		//     end-to-end.
		//
		//   user_input_*
		//     inbound-only by definition.
		return true
	}
}

// convertText handles assistant_gen_text.
//
// Two different provider deltas land on this one block kind: a text delta populates
// .Text, and a citations delta populates .ClaudeExtension.Citations while leaving .Text
// EMPTY. A "same kind means delta" rule turns the citation into text-delta{delta:""} and
// the citation itself vanishes. So the sub-field decides, not the kind.
func convertText(
	b *schema.ContentBlock, index int, st *ConvState,
	yield func(aisdk.StepEvent, error) bool,
) bool {
	text := ""
	if b.AssistantGenText != nil {
		text = b.AssistantGenText.Text
	}
	citations := textCitations(b)

	// A citations-only frame carries no text and must not open or delta a text block.
	if text == "" && len(citations) > 0 {
		return emitCitations(citations, yield)
	}

	if !openIfNeeded(st, index, blockText, yield) {
		return false
	}
	// An empty text delta is dropped rather than emitted: it would be a no-op chunk on
	// the wire, and an empty delta is how a mishandled signature or citation shows up.
	if text == "" {
		return true
	}
	return yield(aisdk.StepEvent{
		Type:      aisdk.StepEventTextDelta,
		BlockID:   st.Block(index).id,
		TextDelta: text,
	}, nil)
}

// convertReasoning handles reasoning, whose two deltas split the same way as text: a
// thinking delta populates .Text, and a signature delta populates .Signature with .Text
// EMPTY. The signature is accumulated onto the block and emitted on reasoning-end.
func convertReasoning(
	b *schema.ContentBlock, index int, st *ConvState,
	yield func(aisdk.StepEvent, error) bool,
) bool {
	var text, signature string
	if b.Reasoning != nil {
		text, signature = b.Reasoning.Text, b.Reasoning.Signature
	}

	// Opened even for a signature-only frame. The Phase 00 spike measured that eino will
	// transport a signature with no preceding thinking delta, so the block has to be
	// opened lazily on first sight of the index rather than assumed already open.
	if !openIfNeeded(st, index, blockReasoning, yield) {
		return false
	}
	if signature != "" {
		st.Block(index).signature += signature
	}
	if text == "" {
		return true
	}
	return yield(aisdk.StepEvent{
		Type:           aisdk.StepEventReasoningDelta,
		BlockID:        st.Block(index).id,
		ReasoningDelta: text,
	}, nil)
}

// convertFunctionToolCall handles a server-dispatched tool call.
//
// The start frame carries {CallID, Name}; every delta frame carries only Arguments, with
// CallID and Name empty. Identity therefore comes from ConvState via the index — see
// blockState's doc comment.
func convertFunctionToolCall(
	b *schema.ContentBlock, index int, st *ConvState,
	yield func(aisdk.StepEvent, error) bool,
) bool {
	call := b.FunctionToolCall
	if call == nil {
		return true
	}

	existing := st.Block(index)
	if existing == nil || existing.kind != blockToolCall {
		if existing != nil && !closeBlock(st, index, yield) {
			return false
		}
		bs := st.Open(index, blockToolCall)
		bs.callID, bs.toolName = call.CallID, call.Name
		if !yield(aisdk.StepEvent{
			Type:             aisdk.StepEventToolCallStart,
			ToolCallID:       bs.callID,
			ToolCallName:     bs.toolName,
			ProviderExecuted: boolPtr(false),
		}, nil) {
			return false
		}
		existing = bs
	}

	// A start frame may also carry arguments; a delta frame carries only arguments.
	if call.CallID != "" && existing.callID == "" {
		existing.callID = call.CallID
	}
	if call.Name != "" && existing.toolName == "" {
		existing.toolName = call.Name
	}
	if call.Arguments == "" {
		return true
	}
	existing.args.WriteString(call.Arguments)
	return yield(aisdk.StepEvent{
		Type:              aisdk.StepEventToolCallDelta,
		ToolCallID:        existing.callID,
		ToolCallName:      existing.toolName,
		ToolCallArgsDelta: call.Arguments,
	}, nil)
}

// convertFunctionToolResult handles a tool result.
//
// Content is []*FunctionToolResultContentBlock — text/image/audio/video/file — not a
// scalar, so it needs a structural conversion rather than a cast. Only the text parts
// become the result's string output; media parts are collected separately.
func convertFunctionToolResult(
	b *schema.ContentBlock, st *ConvState,
	yield func(aisdk.StepEvent, error) bool,
) bool {
	res := b.FunctionToolResult
	if res == nil {
		return true
	}
	return yield(aisdk.StepEvent{
		Type: aisdk.StepEventToolResult,
		ToolResult: &aisdk.StepToolResult{
			ID:      res.CallID,
			Name:    res.Name,
			Output:  flattenResultText(res.Content),
			Content: convertResultContent(res.Content),
		},
		ProviderExecuted: boolPtr(false),
	}, nil)
}

// convertServerToolCall handles a provider-run tool call.
//
// Structurally different from a function tool call in exactly the two fields v7 keys on.
// ServerToolCall.Arguments is `any`, not a JSON string, so there is no partial-JSON stream
// and no delta path — the input is marshalled once. And CallID is documented as "Empty if
// not provided by the model server", which would make two web_search calls in one turn
// collapse into a single rendered part, silently.
func convertServerToolCall(
	b *schema.ContentBlock, index int, st *ConvState,
	yield func(aisdk.StepEvent, error) bool,
) bool {
	call := b.ServerToolCall
	if call == nil {
		return true
	}
	callID := call.CallID
	if callID == "" {
		callID = st.SynthesizeCallID(index)
	}

	if !yield(aisdk.StepEvent{
		Type:             aisdk.StepEventToolCallStart,
		ToolCallID:       callID,
		ToolCallName:     call.Name,
		ProviderExecuted: boolPtr(true),
	}, nil) {
		return false
	}
	return yield(aisdk.StepEvent{
		Type:             aisdk.StepEventToolCallReady,
		ToolCallID:       callID,
		ToolCallName:     call.Name,
		ToolInput:        call.Arguments,
		ProviderExecuted: boolPtr(true),
	}, nil)
}

func convertServerToolResult(
	b *schema.ContentBlock, index int, st *ConvState,
	yield func(aisdk.StepEvent, error) bool,
) bool {
	res := b.ServerToolResult
	if res == nil {
		return true
	}
	callID := res.CallID
	if callID == "" {
		callID = st.SynthesizeCallID(index)
	}
	return yield(aisdk.StepEvent{
		Type: aisdk.StepEventToolResult,
		ToolResult: &aisdk.StepToolResult{
			ID:     callID,
			Name:   res.Name,
			Output: marshalToString(res.Content),
		},
		ProviderExecuted: boolPtr(true),
	}, nil)
}

// convertToolSearchResult maps a client-side tool-search result to a tool result.
//
// Imperfect and knowingly so: v7's source-url/source-document parts would render search
// hits better, but they are source parts rather than tool results and the shapes do not
// line up. Revisit if a real use case appears.
func convertToolSearchResult(b *schema.ContentBlock, yield func(aisdk.StepEvent, error) bool) bool {
	res := b.ToolSearchFunctionToolResult
	if res == nil {
		return true
	}
	return yield(aisdk.StepEvent{
		Type: aisdk.StepEventToolResult,
		ToolResult: &aisdk.StepToolResult{
			ID: res.CallID, Name: res.Name, Output: res.String(),
		},
	}, nil)
}

// openIfNeeded opens a block at index, closing a different-kind block already there.
func openIfNeeded(
	st *ConvState, index int, kind blockKind,
	yield func(aisdk.StepEvent, error) bool,
) bool {
	if b := st.Block(index); b != nil {
		if b.kind == kind {
			return true
		}
		// One index changing kind mid-turn is not something a provider does, but if it
		// happens the old block must be closed rather than silently reinterpreted.
		if !closeBlock(st, index, yield) {
			return false
		}
	}
	bs := st.Open(index, kind)

	switch kind {
	case blockText:
		return yield(aisdk.StepEvent{Type: aisdk.StepEventTextStart, BlockID: bs.id}, nil)
	case blockReasoning:
		return yield(aisdk.StepEvent{Type: aisdk.StepEventReasoningStart, BlockID: bs.id}, nil)
	}
	return true
}

// textCitations extracts Claude citations, which ride on the text block's extension.
func textCitations(b *schema.ContentBlock) []string {
	if b.AssistantGenText == nil || b.AssistantGenText.ClaudeExtension == nil {
		return nil
	}
	var out []string
	for _, c := range b.AssistantGenText.ClaudeExtension.Citations {
		if c == nil {
			continue
		}
		switch {
		case c.WebSearchResultLocation != nil && c.WebSearchResultLocation.URL != "":
			out = append(out, c.WebSearchResultLocation.URL)
		case c.PageLocation != nil && c.PageLocation.DocumentTitle != "":
			out = append(out, c.PageLocation.DocumentTitle)
		case c.CharLocation != nil && c.CharLocation.DocumentTitle != "":
			out = append(out, c.CharLocation.DocumentTitle)
		}
	}
	return out
}

// emitCitations turns citations into source events, which is the nearest v7 part.
func emitCitations(citations []string, yield func(aisdk.StepEvent, error) bool) bool {
	for _, c := range citations {
		src := &aisdk.Source{ID: c, URL: c, Title: c}
		if !strings.HasPrefix(c, "http") {
			// A document title is not a URL; carry it as the title only, since source-url
			// requires a url the client would otherwise render as a broken link.
			src = &aisdk.Source{ID: c, Title: c}
		}
		if !yield(aisdk.StepEvent{Type: aisdk.StepEventSource, Source: src}, nil) {
			return false
		}
	}
	return true
}

// flattenResultText joins the text parts of a multimodal tool result.
func flattenResultText(blocks []*schema.FunctionToolResultContentBlock) string {
	var sb strings.Builder
	for _, b := range blocks {
		if b == nil || b.Type != schema.FunctionToolResultContentBlockTypeText || b.Text == nil {
			continue
		}
		sb.WriteString(b.Text.Text)
	}
	return sb.String()
}

// convertResultContent maps the multimodal parts of a tool result.
func convertResultContent(blocks []*schema.FunctionToolResultContentBlock) []aisdk.ToolResultContent {
	var out []aisdk.ToolResultContent
	for _, b := range blocks {
		if b == nil {
			continue
		}
		switch b.Type {
		case schema.FunctionToolResultContentBlockTypeText:
			if b.Text != nil {
				out = append(out, aisdk.ToolResultContent{
					Type: aisdk.ToolResultContentTypeText, Text: b.Text.Text,
				})
			}
		case schema.FunctionToolResultContentBlockTypeImage:
			if b.Image != nil {
				out = append(out, aisdk.ToolResultContent{
					Type: aisdk.ToolResultContentTypeFile, MediaType: b.Image.MIMEType,
				})
			}
		case schema.FunctionToolResultContentBlockTypeFile:
			if b.File != nil {
				out = append(out, aisdk.ToolResultContent{
					Type: aisdk.ToolResultContentTypeFile, MediaType: b.File.MIMEType,
				})
			}
		}
	}
	return out
}

func marshalToString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
