package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/open-ai-sdk/ai-go/internal/safego"
)

type historyApprovalResponse struct {
	approved  bool
	reason    string
	signature string
}

type historyToolResult struct {
	name     string
	output   string
	receipt  string
	approved bool
}

type historyApprovalCall struct {
	tc            toolCallState
	approvalID    string
	ordinal       int
	result        *historyToolResult
	response      *historyApprovalResponse
	canonicalArgs string
}

// resumeToolApprovals validates the entire approval envelope before invoking
// any tool. It then replaces client-only response parts with provider-facing
// tool results. Correlation and provenance come from signed message history;
// the runtime retains no per-request state between invocations.
func resumeToolApprovals(r *run, params RunParams, history []Message) ([]Message, error) {
	if len(params.ToolApproval) == 0 {
		return history, nil
	}

	calls := make([]*historyApprovalCall, 0)
	callsByApprovalID := make(map[string]*historyApprovalCall)

	for _, message := range history {
		messageHasApproval := false
		messageHasOther := false
		for _, part := range message.Content {
			if part.Type == "tool_approval_response" {
				messageHasApproval = true
			} else {
				messageHasOther = true
			}
		}
		if messageHasApproval && message.Role != "user" {
			return nil, errors.New("agent: approval responses must be in a user message")
		}
		if messageHasApproval && messageHasOther {
			return nil, errors.New("agent: an approval-response message cannot contain other content")
		}

		for _, part := range message.Content {
			switch part.Type {
			case "tool_call":
				if message.Role != "assistant" {
					return nil, fmt.Errorf("agent: tool call %q must be in an assistant message", part.ToolCallID)
				}
				if part.ToolCallID == "" {
					return nil, errors.New("agent: tool call ID is empty in approval history")
				}
				call := &historyApprovalCall{
					tc: toolCallState{
						id: part.ToolCallID, name: part.ToolCallName,
						args: string(part.ToolCallArgs), thoughtSignature: part.ThoughtSignature,
					},
					approvalID: part.ToolApprovalID,
					ordinal:    len(calls),
				}
				calls = append(calls, call)
				if call.approvalID != "" {
					if _, duplicate := callsByApprovalID[call.approvalID]; duplicate {
						return nil, fmt.Errorf("agent: duplicate approval ID %q in history", call.approvalID)
					}
					callsByApprovalID[call.approvalID] = call
				}

			case "tool_result":
				if message.Role != "tool" {
					return nil, fmt.Errorf("agent: tool result %q must be in a tool message", part.ToolResultID)
				}
				call := latestUnresolvedToolCall(calls, part.ToolResultID)
				if call == nil {
					return nil, fmt.Errorf("agent: tool result %q precedes its tool call or duplicates a result", part.ToolResultID)
				}
				if part.ToolResultName != call.tc.name {
					return nil, fmt.Errorf("agent: tool result %q names %q, want %q", part.ToolResultID, part.ToolResultName, call.tc.name)
				}
				call.result = &historyToolResult{
					name: part.ToolResultName, output: part.ToolResultOutput,
					receipt:  part.ToolResultApprovalSignature,
					approved: part.ToolResultApprovalApproved,
				}

			case "tool_approval_response":
				call := callsByApprovalID[part.ToolApprovalID]
				if call == nil {
					// Compatibility for histories created before approval IDs were
					// distinct: correlate the latest unanswered call by tool-call ID.
					call = latestUnansweredToolCall(calls, part.ToolApprovalID)
				}
				if call == nil {
					return nil, fmt.Errorf("agent: approval response %q precedes or has no matching tool call", part.ToolApprovalID)
				}
				if call.response != nil {
					return nil, fmt.Errorf("agent: duplicate approval response %q", part.ToolApprovalID)
				}
				call.response = &historyApprovalResponse{
					approved:  part.ToolApprovalApproved,
					reason:    part.ToolApprovalReason,
					signature: part.ToolApprovalSignature,
				}
			}
		}
	}

	activeTools := params.Request.Tools
	if activeTools == nil && params.Tools != nil {
		activeTools = params.Tools.Definitions
	}
	stepTools := toolSetForStep(params.Tools, activeTools)
	definitions := make(map[*historyApprovalCall]ToolDefinition)
	grants := make([]ApprovalGrant, 0)
	grantsByCall := make(map[*historyApprovalCall]ApprovalGrant)

	for _, call := range calls {
		tc := call.tc
		policy := params.ToolApproval[tc.name]
		requiresApproval := false
		if policy != nil {
			canonical, _, err := canonicalizeApprovalInput(tc.args)
			if err != nil {
				return nil, fmt.Errorf("agent: canonicalize approval input for %q: %w", tc.id, err)
			}
			call.canonicalArgs = canonical
			tc.args = canonical
			call.tc = tc
			requiresApproval = policy(tc.name, tc.args)
		}
		if !requiresApproval {
			if call.response != nil {
				return nil, fmt.Errorf("agent: tool call %q is not approval-gated by this request", tc.id)
			}
			continue
		}

		approvalID := call.approvalID
		if approvalID == "" {
			approvalID = tc.id
		}
		request := ApprovalRequest{
			ApprovalID: approvalID, ToolCallID: tc.id, ToolName: tc.name, Args: tc.args,
		}
		if call.result != nil {
			if call.response != nil {
				return nil, fmt.Errorf("agent: approval response %q also carries a completed tool result", approvalID)
			}
			if err := verifyApprovalResult(
				r.approvalKey, request, call.result.approved, call.result.output, call.result.receipt,
			); err != nil {
				return nil, fmt.Errorf("agent: approval-gated tool result %q is unauthenticated: %w", tc.id, err)
			}
			continue
		}
		if call.response == nil {
			return nil, fmt.Errorf("agent: pending tool call %q has no approval response", tc.id)
		}

		grant, err := verifyApprovalRequest(r.approvalKey, request, call.response.signature)
		if err != nil {
			return nil, fmt.Errorf("agent: approval response %q: %w", approvalID, err)
		}
		call.canonicalArgs = grant.canonicalArgs
		tc.args = grant.canonicalArgs
		call.tc = tc
		definition, err := validateToolCall(stepTools, tc)
		if err != nil {
			return nil, fmt.Errorf("agent: approved tool call %q is invalid: %w", tc.id, err)
		}
		definitions[call] = definition
		if call.response.approved {
			grants = append(grants, grant)
			grantsByCall[call] = grant
		}
	}

	var reservation ApprovalReservation
	if len(grants) > 0 {
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}
		if r.approvalReplayGuard == nil {
			return nil, errors.New("agent: ToolApprovalReplayGuard is required to execute an approved history response")
		}
		var err error
		reservation, err = r.approvalReplayGuard.ReserveApprovals(r.ctx, grants)
		if err != nil {
			return nil, fmt.Errorf("agent: reserve approval capability: %w", err)
		}
		defer func() { _ = reservation.Release(context.Background()) }()
	}

	generated := make(map[int]Message)
	for _, call := range calls {
		if call.response == nil {
			continue
		}
		tc := call.tc
		definition := definitions[call]
		var result *ToolResult
		var terminalErr error
		if !call.response.approved {
			r.emitObserved(StepEvent{
				Type: StepEventToolOutputDenied, ToolCallID: tc.id, ApprovalID: approvalIDForCall(call),
			})
			result = deniedToolResult(tc, call.response.reason, nil)
		} else {
			if err := r.ctx.Err(); err != nil {
				return nil, err
			}
			if !r.emitObserved(StepEvent{
				Type: StepEventToolCallReady, ToolCallID: tc.id, ToolCallName: tc.name,
				ToolCallArgsDelta: tc.args, ThoughtSignature: tc.thoughtSignature,
				ApprovalID: approvalIDForCall(call),
			}) {
				return nil, r.ctx.Err()
			}
			result, terminalErr = executeReservedToolCall(
				r, stepTools, tc, definition, reservation, grantsByCall[call],
			)
		}

		modelOutput, transformErr := transformToolOutput(r, definition, result)
		if transformErr != nil {
			terminalErr = errors.Join(terminalErr, transformErr)
		}
		result.ModelOutput = modelOutput
		result.ModelOutputSet = true
		request := ApprovalRequest{
			ApprovalID: approvalIDForCall(call), ToolCallID: tc.id, ToolName: tc.name, Args: tc.args,
		}
		receipt, err := signApprovalResult(r.approvalKey, request, call.response.approved, modelOutput)
		if err != nil {
			return nil, err
		}
		result.ApprovalID = request.ApprovalID
		result.ApprovalSignature = receipt
		result.ApprovalApproved = call.response.approved
		if !r.emitObserved(StepEvent{Type: StepEventToolResult, ToolResult: result, ApprovalID: request.ApprovalID}) {
			return nil, r.ctx.Err()
		}
		generated[call.ordinal] = toolResultHistoryMessage(result, modelOutput)
		if terminalErr != nil {
			return nil, terminalErr
		}
	}

	resumed := make([]Message, 0, len(history)+len(generated))
	ordinal := 0
	for _, message := range history {
		kept := make([]ContentPart, 0, len(message.Content))
		messageCallOrdinals := make([]int, 0)
		for _, part := range message.Content {
			if part.Type == "tool_approval_response" {
				continue
			}
			if part.Type == "tool_call" {
				call := calls[ordinal]
				if call.canonicalArgs != "" {
					part.ToolCallArgs = []byte(call.canonicalArgs)
				}
				messageCallOrdinals = append(messageCallOrdinals, ordinal)
				ordinal++
			}
			kept = append(kept, part)
		}
		if len(kept) > 0 {
			message.Content = kept
			resumed = append(resumed, message)
		}
		for _, callOrdinal := range messageCallOrdinals {
			if result, ok := generated[callOrdinal]; ok {
				resumed = append(resumed, result)
			}
		}
	}
	return resumed, nil
}

func executeReservedToolCall(
	r *run,
	tools *ToolSet,
	tc toolCallState,
	definition ToolDefinition,
	reservation ApprovalReservation,
	grant ApprovalGrant,
) (result *ToolResult, err error) {
	defer safego.Recover(r.logger, func(recovered error) {
		err = errors.Join(err, recovered)
		result = &ToolResult{
			ID: tc.id, Name: tc.name, Args: tc.args,
			Output: invalidToolCallOutput(tc, recovered),
			Error:  classifyToolError(tc.name, recovered),
		}
	}, "phase", "resumed-tool-exec", "tool", tc.name)
	defer func() {
		if completeErr := reservation.Complete(context.Background(), grant); completeErr != nil {
			err = errors.Join(err, fmt.Errorf("agent: complete approval capability: %w", completeErr))
		}
	}()
	result = executeToolCall(r.ctx, tools, tc, definition)
	return result, nil
}

func approvalIDForCall(call *historyApprovalCall) string {
	if call.approvalID != "" {
		return call.approvalID
	}
	return call.tc.id
}

func latestUnresolvedToolCall(calls []*historyApprovalCall, toolCallID string) *historyApprovalCall {
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].tc.id == toolCallID && calls[i].result == nil {
			return calls[i]
		}
	}
	return nil
}

func latestUnansweredToolCall(calls []*historyApprovalCall, toolCallID string) *historyApprovalCall {
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].approvalID == "" && calls[i].tc.id == toolCallID && calls[i].response == nil {
			return calls[i]
		}
	}
	return nil
}
