package agent

import "encoding/json"

func buildInitialHistory(req Request) []Message {
	msgs := make([]Message, 0, len(req.Messages)+1)
	if req.Instructions != "" {
		msgs = append(msgs, Message{Role: "system", Content: []ContentPart{{Type: "text", Text: req.Instructions}}})
	}
	msgs = append(msgs, req.Messages...)
	return msgs
}

func buildAssistantToolCallMessage(messageID, text, reasoning string, calls []toolCallState) Message {
	parts := make([]ContentPart, 0, 2+len(calls))
	if reasoning != "" {
		parts = append(parts, ContentPart{Type: "reasoning", ReasoningText: reasoning})
	}
	if text != "" {
		parts = append(parts, ContentPart{Type: "text", Text: text})
	}
	for _, tc := range calls {
		parts = append(parts, ContentPart{
			Type:             "tool_call",
			ToolCallID:       tc.id,
			ToolCallName:     tc.name,
			ToolCallArgs:     json.RawMessage(tc.args),
			ThoughtSignature: tc.thoughtSignature,
		})
	}
	return Message{ID: messageID, Role: "assistant", Content: parts}
}

func buildToolResultMessageWithApproval(
	toolCallID, toolName, output, signature string,
	approved bool,
) Message {
	return Message{
		Role: "tool",
		Content: []ContentPart{{
			Type:                        "tool_result",
			ToolResultID:                toolCallID,
			ToolResultName:              toolName,
			ToolResultOutput:            output,
			ToolResultApprovalSignature: signature,
			ToolResultApprovalApproved:  approved,
		}},
	}
}

func mergeProviderOptions(base, override map[string]any) map[string]any {
	if base == nil {
		return override
	}
	merged := make(map[string]any, len(base)+len(override))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range override {
		merged[k] = v
	}
	return merged
}

func filterTools(tools []ToolDefinition, active []string) []ToolDefinition {
	if len(active) == 0 {
		return nil
	}
	set := make(map[string]bool, len(active))
	for _, name := range active {
		set[name] = true
	}
	var filtered []ToolDefinition
	for _, t := range tools {
		if set[t.Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

func emitOnStepEnd(
	cb *lifecycleCallbacks,
	step int,
	toolCalls []ToolCallInfo,
	toolResults []ToolResult,
	sr streamResult,
) {
	if cb == nil || cb.OnStepEnd == nil {
		return
	}
	cb.OnStepEnd(stepEndEvent{
		StepNumber:       step,
		MessageID:        sr.messageID,
		Text:             sr.text,
		Reasoning:        sr.reasoning,
		ToolCalls:        snapshotToolCallInfos(toolCalls),
		ToolResults:      snapshotToolResults(toolResults),
		FinishReason:     sr.finish,
		Usage:            snapshotUsage(sr.usage),
		ProviderMetadata: snapshotJSONMap(sr.providerMeta),
		Warnings:         append([]Warning(nil), sr.warnings...),
	})
}

func emitOnEnd(cb *lifecycleCallbacks, steps []StepResultInfo, sr streamResult) {
	if cb == nil || cb.OnEnd == nil {
		return
	}
	var totalText, totalReasoning string
	var totalUsage Usage
	var lastFinish FinishReason
	var lastMessageID string
	var lastMeta map[string]any
	for _, s := range steps {
		totalText += s.Text
		totalReasoning += s.Reasoning
		lastFinish = s.FinishReason
		if s.MessageID != "" {
			lastMessageID = s.MessageID
		}
		if s.Usage != nil {
			totalUsage.Accumulate(*s.Usage)
		}
		if s.ProviderMetadata != nil {
			lastMeta = s.ProviderMetadata
		}
	}
	if sr.finish != "" {
		lastFinish = sr.finish
	}
	if sr.messageID != "" {
		lastMessageID = sr.messageID
	}
	if sr.providerMeta != nil {
		lastMeta = sr.providerMeta
	}
	cb.OnEnd(endEvent{
		MessageID:        lastMessageID,
		Text:             totalText,
		Reasoning:        totalReasoning,
		Steps:            snapshotPrepareStepInfos(steps),
		Usage:            *snapshotUsage(&totalUsage),
		FinishReason:     lastFinish,
		ProviderMetadata: snapshotJSONMap(lastMeta),
	})
}
