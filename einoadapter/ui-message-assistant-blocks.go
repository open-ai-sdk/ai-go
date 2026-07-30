package einoadapter

import (
	"encoding/json"

	"github.com/cloudwego/eino/schema"

	"github.com/open-ai-sdk/ai-go/aisdk"
)

// assistantMessages converts one assistant UI message into the messages a model needs.
//
// An assistant message becomes 1..N assistant messages plus their tool-result messages,
// grouped by step-start. The grouping is half of a contract with the outbound producer:
// it decides where `start-step` goes on the wire, and this decides where a step begins
// coming back. If the two disagree, tool calls pair with the wrong assistant turn.
func assistantMessages(m *aisdk.UIMessage, cfg convertConfig) ([]*schema.AgenticMessage, error) {
	var out []*schema.AgenticMessage

	block := make([]*aisdk.UIMessagePart, 0, len(m.Parts))
	flush := func() error {
		if len(block) == 0 {
			return nil
		}
		msgs, err := convertAssistantBlock(block, cfg)
		if err != nil {
			return err
		}
		out = append(out, msgs...)
		block = block[:0]
		return nil
	}

	for i := range m.Parts {
		p := &m.Parts[i]
		if p.Type == aisdk.UIPartStepStart {
			// A step boundary, not content. It closes the previous block.
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}
		block = append(block, p)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return out, nil
}

// convertAssistantBlock turns one step's parts into an assistant message plus the
// user-role tool-result messages that follow it.
func convertAssistantBlock(parts []*aisdk.UIMessagePart, cfg convertConfig) ([]*schema.AgenticMessage, error) {
	var content []*schema.ContentBlock

	for _, p := range parts {
		switch {
		case p.Type == aisdk.UIPartText && p.Text != "":
			content = append(content, schema.NewContentBlock(
				&schema.AssistantGenText{Text: p.Text}))

		case p.Type == aisdk.UIPartReasoning:
			r := &schema.Reasoning{Text: p.Text}
			// The signature the model needs echoed back rides on provider metadata,
			// which is where the outbound side put it.
			if sig, ok := p.ProviderMetadata["signature"].(string); ok {
				r.Signature = sig
			}
			content = append(content, schema.NewContentBlock(r))

		case p.IsToolPart():
			content = append(content, toolCallBlocks(p)...)

		case p.IsDataPart():
			if cfg.convertDataPart != nil {
				if b := cfg.convertDataPart(p.DataNameOf(), p.Data); b != nil {
					content = append(content, b)
				}
			}
		}
		// file / reasoning-file / source-* on an assistant message are dropped: Eino's
		// assistant role accepts no user-input blocks, and assistant_gen_image is
		// inbound-rejected by the provider.
	}

	var out []*schema.AgenticMessage
	if len(content) > 0 {
		out = append(out, &schema.AgenticMessage{
			Role: schema.AgenticRoleTypeAssistant, ContentBlocks: content,
		})
	}
	return append(out, toolResultMessages(parts)...), nil
}

// toolCallBlocks renders the assistant-side blocks for one tool part.
func toolCallBlocks(p *aisdk.UIMessagePart) []*schema.ContentBlock {
	state := p.ToolStateOf()
	// input-streaming is a partial call the client rendered; the model never issued it in
	// that form, so replaying it would invent a call.
	if state == aisdk.UIToolInputStreaming {
		return nil
	}

	// output-error keeps the input that failed, falling back to rawInput — which is where
	// the client stores an unparseable input, since the processor moves it there.
	input := p.Input
	if state == aisdk.UIToolOutputError && len(input) == 0 {
		input = p.RawInput
	}

	if p.ProviderExecuted != nil && *p.ProviderExecuted {
		return providerExecutedBlocks(p, input)
	}

	return []*schema.ContentBlock{schema.NewContentBlock(&schema.FunctionToolCall{
		CallID: p.ToolCallID, Name: p.ToolNameOf(), Arguments: rawToString(input),
	})}
}

// providerExecutedBlocks renders a tool the model provider ran itself.
//
// Its result belongs in the assistant content, not in a separate tool message — the
// reference skips those in the tool message specifically to avoid orphaned outputs
// (convert-to-model-messages.ts:337-345). Emitting both would leave the provider with a
// result it cannot pair.
func providerExecutedBlocks(p *aisdk.UIMessagePart, input json.RawMessage) []*schema.ContentBlock {
	var inputValue any
	if len(input) > 0 {
		_ = json.Unmarshal(input, &inputValue)
	}

	blocks := []*schema.ContentBlock{schema.NewContentBlock(&schema.ServerToolCall{
		CallID: p.ToolCallID, Name: p.ToolNameOf(), Arguments: inputValue,
	})}

	switch p.ToolStateOf() {
	case aisdk.UIToolOutputAvailable:
		var outputValue any
		if len(p.Output) > 0 {
			_ = json.Unmarshal(p.Output, &outputValue)
		}
		blocks = append(blocks, schema.NewContentBlock(&schema.ServerToolResult{
			CallID: p.ToolCallID, Name: p.ToolNameOf(), Content: outputValue,
		}))
	case aisdk.UIToolOutputError:
		blocks = append(blocks, schema.NewContentBlock(&schema.ServerToolResult{
			CallID: p.ToolCallID, Name: p.ToolNameOf(),
			Content: aisdk.EncodeErrorText(p.ErrorText),
		}))
	}
	return blocks
}

// toolResultMessages renders the user-role messages carrying tool results.
//
// User role, one message per result, no pre-merging — see ToAgenticMessages' comment.
func toolResultMessages(parts []*aisdk.UIMessagePart) []*schema.AgenticMessage {
	var out []*schema.AgenticMessage

	for _, p := range parts {
		if !p.IsToolPart() {
			continue
		}
		// A provider-executed tool's result is already in the assistant content above.
		// The exception is an approval decision, which the model still has to see.
		providerExecuted := p.ProviderExecuted != nil && *p.ProviderExecuted
		_, answered := p.ApprovalDecision()
		if providerExecuted && !answered {
			continue
		}

		text, ok := toolResultText(p)
		if !ok {
			continue
		}
		out = append(out, &schema.AgenticMessage{
			Role: schema.AgenticRoleTypeUser,
			ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(
				&schema.FunctionToolResult{
					CallID: p.ToolCallID, Name: p.ToolNameOf(),
					Content: []*schema.FunctionToolResultContentBlock{{
						Type: schema.FunctionToolResultContentBlockTypeText,
						Text: &schema.UserInputText{Text: text},
					}},
				})},
		})
	}
	return out
}

// toolResultText renders the result payload, covering BOTH denial paths.
//
// The two are genuinely different and the reference keeps them apart:
//
//   - approval-responded with approved:false → execution-denied. The user refused; the
//     tool never ran (convert-to-model-messages.ts:320-336).
//   - output-denied → error-text. The denial already happened on an earlier turn and is
//     being replayed as history (:346-362).
//
// Collapsing them would tell the model a tool failed when the user declined it, or that a
// user is being asked again about something already settled.
func toolResultText(p *aisdk.UIMessagePart) (string, bool) {
	approved, answered := p.ApprovalDecision()

	if p.ToolStateOf() == aisdk.UIToolApprovalResponded && answered && !approved {
		return aisdk.EncodeExecutionDenied(p.Approval.Reason), true
	}

	switch p.ToolStateOf() {
	case aisdk.UIToolOutputDenied:
		reason := ""
		if p.Approval != nil {
			reason = p.Approval.Reason
		}
		return aisdk.EncodeErrorText(reason), true

	case aisdk.UIToolOutputError:
		return aisdk.EncodeErrorText(p.ErrorText), true

	case aisdk.UIToolOutputAvailable:
		return rawToString(p.Output), true
	}
	return "", false
}

// rawToString renders raw JSON for a field the provider expects as a string. A JSON string
// is unquoted, since a tool returning "4" should reach the model as 4 rather than "\"4\"".
func rawToString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}
