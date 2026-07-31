package agent

import (
	"fmt"
)

// resumeToolApprovals consumes approval-response parts from message history,
// executes (or denies) their matching pending tool calls, and replaces the
// client-only response parts with provider-facing tool-result messages. All
// correlation state is reconstructed from the request; the runtime retains
// nothing between invocations.
func resumeToolApprovals(r *run, params RunParams, history []Message) ([]Message, error) {
	calls := make(map[string]toolCallState)
	completed := make(map[string]bool)
	hasResponses := false
	for _, message := range history {
		for _, part := range message.Content {
			switch part.Type {
			case "tool_call":
				calls[part.ToolCallID] = toolCallState{
					id:               part.ToolCallID,
					name:             part.ToolCallName,
					args:             string(part.ToolCallArgs),
					thoughtSignature: part.ThoughtSignature,
				}
			case "tool_result":
				completed[part.ToolResultID] = true
			case "tool_approval_response":
				hasResponses = true
			}
		}
	}
	if !hasResponses {
		return history, nil
	}

	activeTools := params.Request.Tools
	if activeTools == nil && params.Tools != nil {
		activeTools = params.Tools.Definitions
	}
	stepTools := toolSetForStep(params.Tools, activeTools)
	resumed := make([]Message, 0, len(history))
	for _, message := range history {
		kept := make([]ContentPart, 0, len(message.Content))
		results := make([]Message, 0, 1)
		for _, part := range message.Content {
			if part.Type != "tool_approval_response" {
				kept = append(kept, part)
				continue
			}

			tc, ok := calls[part.ToolApprovalID]
			if !ok {
				return nil, fmt.Errorf("agent: approval response %q has no matching tool call", part.ToolApprovalID)
			}
			if completed[tc.id] {
				return nil, fmt.Errorf("agent: tool call %q already has a result", tc.id)
			}

			def, validationErr := validateToolCall(stepTools, tc)
			var result *ToolResult
			if validationErr != nil {
				r.emit(StepEvent{
					Type:              StepEventToolCallInvalid,
					ToolCallID:        tc.id,
					ToolCallName:      tc.name,
					ToolCallArgsDelta: tc.args,
				})
				result = &ToolResult{
					ID: tc.id, Name: tc.name, Args: tc.args,
					Output: invalidToolCallOutput(tc, validationErr),
					Error:  classifyToolError(tc.name, validationErr),
				}
			} else if !part.ToolApprovalApproved {
				r.emit(StepEvent{Type: StepEventToolOutputDenied, ToolCallID: tc.id})
				result = deniedToolResult(tc, part.ToolApprovalReason, nil)
			} else {
				r.emit(StepEvent{
					Type:              StepEventToolCallReady,
					ToolCallID:        tc.id,
					ToolCallName:      tc.name,
					ToolCallArgsDelta: tc.args,
					ThoughtSignature:  tc.thoughtSignature,
				})
				result = executeToolCall(r.ctx, stepTools, tc, def)
			}

			if !r.emit(StepEvent{Type: StepEventToolResult, ToolResult: result}) {
				return nil, r.ctx.Err()
			}
			modelOutput := result.Output
			if def.ToModelOutput != nil {
				modelOutput = def.ToModelOutput(result.Output)
			}
			results = append(results, buildToolResultMessage(tc.id, tc.name, modelOutput))
			completed[tc.id] = true
		}
		if len(kept) > 0 {
			message.Content = kept
			resumed = append(resumed, message)
		}
		resumed = append(resumed, results...)
	}
	return resumed, nil
}
