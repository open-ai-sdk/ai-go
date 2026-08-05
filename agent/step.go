package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/internal/safego"
	"github.com/open-ai-sdk/ai-go/internal/tracing"
	"github.com/open-ai-sdk/ai-go/tool"
	"golang.org/x/sync/errgroup"
)

func executeToolCalls(
	r *run,
	tools *ToolSet,
	prepared []preparedToolCall,
	history *[]Message,
	approval map[string]func(string, string) bool, approver ApprovalResponder,
) (toolNames []string, stepToolCalls []ToolCallInfo, stepToolResults []ToolResult, controlErr error) {
	toolNames = make([]string, 0, len(prepared))
	for _, preparedCall := range prepared {
		tc := preparedCall.tc
		if preparedCall.controlErr != nil {
			controlErr = preferControlErr(controlErr, preparedCall.controlErr)
			break
		}
		if preparedCall.invalidErr != nil {
			if !r.emitObserved(StepEvent{
				Type:              StepEventToolCallInvalid,
				ToolCallID:        tc.id,
				ToolCallName:      tc.name,
				ToolCallArgsDelta: tc.args,
			}) {
				return toolNames, stepToolCalls, stepToolResults, r.stopError()
			}
			result := invalidToolResult(tc, preparedCall.invalidErr)
			if !r.emitObserved(StepEvent{Type: StepEventToolResult, ToolResult: result}) {
				return toolNames, stepToolCalls, stepToolResults, r.stopError()
			}
			*history = append(*history, toolResultHistoryMessage(result, result.ModelOutput))
			toolNames = append(toolNames, tc.name)
			stepToolResults = append(stepToolResults, *result)
			continue
		}

		skip := preparedCall.skip
		skipReason := preparedCall.skipReason
		if !skip {
			var hookErr error
			tc, skip, skipReason, hookErr = r.beforeTool(tc)
			if hookErr != nil {
				controlErr = preferControlErr(controlErr, hookErr)
				break
			}
		}

		if !r.emitObserved(StepEvent{
			Type:              StepEventToolCallReady,
			ToolCallID:        tc.id,
			ToolCallName:      tc.name,
			ToolCallArgsDelta: tc.args,
			ThoughtSignature:  tc.thoughtSignature,
		}) {
			return toolNames, stepToolCalls, stepToolResults, r.stopError()
		}
		toolNames = append(toolNames, tc.name)
		stepToolCalls = append(stepToolCalls, ToolCallInfo{
			ID:               tc.id,
			Name:             tc.name,
			Args:             json.RawMessage(tc.args),
			ArgsSet:          true,
			ThoughtSignature: tc.thoughtSignature,
		})

		// A client-executed tool is declared to the model but never run here.
		// The call has already been streamed; suspending now lets the UI run it
		// and return the result in the next request's history. Siblings still
		// run, mirroring how a pending approval is handled below.
		if !skip && preparedCall.def.ClientExecuted {
			if !r.emitObserved(StepEvent{
				Type:              StepEventClientToolRequest,
				ToolCallID:        tc.id,
				ToolCallName:      tc.name,
				ToolCallArgsDelta: tc.args,
			}) {
				return toolNames, stepToolCalls, stepToolResults, r.stopError()
			}
			controlErr = preferControlErr(controlErr, errClientToolPending)
			continue
		}

		var result *ToolResult
		approvalResolved := false
		var approvalErr error
		if skip {
			result = skippedToolResult(tc, skipReason)
		} else {
			result, approvalResolved, approvalErr = approvedToolCall(
				r, r.ctx, tools, tc, preparedCall.def, approval, approver,
			)
		}
		if approvalErr != nil {
			controlErr = preferControlErr(controlErr, approvalErr)
			continue
		}
		result, hookErr := r.afterTool(result)
		if hookErr != nil {
			controlErr = preferControlErr(controlErr, hookErr)
			break
		}
		// Apply ToModelOutput transform for history; event keeps original output.
		// def was resolved once during validation (prepareToolCalls), so no
		// second scan of the tool registry is needed here.
		modelOutput, transformErr := transformToolOutput(r, preparedCall.def, result)
		if transformErr != nil {
			result.ModelOutput = result.Output
			result.ModelOutputSet = true
			if !r.emitObserved(StepEvent{Type: StepEventToolResult, ToolResult: result}) {
				return toolNames, stepToolCalls, stepToolResults, r.stopError()
			}
			controlErr = preferControlErr(controlErr, transformErr)
			continue
		}
		result.ModelOutput = modelOutput
		result.ModelOutputSet = true
		if err := attachApprovalReceipt(r, tc, result, approvalResolved, modelOutput); err != nil {
			controlErr = preferControlErr(controlErr, err)
			continue
		}
		if result.ApprovalID != "" {
			stepToolCalls[len(stepToolCalls)-1].ApprovalID = result.ApprovalID
			stepToolCalls[len(stepToolCalls)-1].ApprovalSignature = result.ApprovalRequestSignature
		}
		stepToolCalls[len(stepToolCalls)-1].Args = json.RawMessage(result.Args)

		if !r.emitObserved(StepEvent{Type: StepEventToolResult, ToolResult: result}) {
			return toolNames, stepToolCalls, stepToolResults, r.stopError()
		}
		updateLatestToolCallArgs(history, result.ID, result.Args)
		*history = append(*history, toolResultHistoryMessage(result, modelOutput))
		stepToolResults = append(stepToolResults, *result)
	}
	return toolNames, stepToolCalls, stepToolResults, controlErr
}

func skippedToolResult(tc toolCallState, reason string) *ToolResult {
	if reason == "" {
		reason = "tool call skipped by hook"
	}
	return &ToolResult{ID: tc.id, Name: tc.name, Args: tc.args, Output: reason, Disposition: aikit.ToolResultSkipped}
}

// executeToolCallsParallel processes tool calls concurrently, bounded by
// maxParallel via errgroup.SetLimit. g.Go always returns nil: sibling tool calls
// continue when one fails and report the failure per-call, so an
// errgroup first-error-cancels policy would silently change that semantic.
// Per-call errors travel through results[i].result.Output instead of through
// g.Wait's return value. errgroup is used here only for concurrency limiting
// and ctx propagation, not for fail-fast error aggregation.
//
//nolint:gocyclo // Parallel execution keeps validation, cancellation, and ordered commit together.
func executeToolCallsParallel(
	r *run,
	tools *ToolSet,
	prepared []preparedToolCall,
	history *[]Message,
	maxParallel int,
	approval map[string]func(string, string) bool, approver ApprovalResponder,
) (
	toolNames []string,
	stepToolCalls []ToolCallInfo,
	stepToolResults []ToolResult,
	controlErr error,
) {
	if maxParallel <= 0 {
		maxParallel = 5
	}

	type indexedResult struct {
		tc          toolCallState
		result      *ToolResult
		modelOutput string
		controlErr  error
		valid       bool
	}

	results := make([]indexedResult, len(prepared))

	g, gctx := errgroup.WithContext(r.ctx)
	g.SetLimit(maxParallel)

	for i, preparedCall := range prepared {
		tc := preparedCall.tc
		def := preparedCall.def
		if preparedCall.controlErr != nil {
			results[i] = indexedResult{tc: tc, valid: true, controlErr: preparedCall.controlErr}
			continue
		}
		if preparedCall.invalidErr != nil {
			results[i] = indexedResult{
				tc: tc, valid: false,
				result: invalidToolResult(tc, preparedCall.invalidErr),
			}
			continue
		}
		skip := preparedCall.skip
		skipReason := preparedCall.skipReason
		if !skip {
			var hookErr error
			tc, skip, skipReason, hookErr = r.beforeTool(tc)
			if hookErr != nil {
				results[i] = indexedResult{tc: tc, valid: true, controlErr: hookErr}
				continue
			}
		}

		// Emit ToolCallReady before execution starts (matches sequential contract).
		if !r.emitObserved(StepEvent{
			Type:              StepEventToolCallReady,
			ToolCallID:        tc.id,
			ToolCallName:      tc.name,
			ToolCallArgsDelta: tc.args,
			ThoughtSignature:  tc.thoughtSignature,
		}) {
			results[i] = indexedResult{tc: tc, valid: true, controlErr: r.stopError()}
			continue
		}

		// Suspend before g.Go so no tool body is ever scheduled for a
		// client-executed tool. Placing this inside the goroutine would race
		// with execution the client owns.
		if !skip && preparedCall.def.ClientExecuted {
			if !r.emitObserved(StepEvent{
				Type:              StepEventClientToolRequest,
				ToolCallID:        tc.id,
				ToolCallName:      tc.name,
				ToolCallArgsDelta: tc.args,
			}) {
				results[i] = indexedResult{tc: tc, valid: true, controlErr: r.stopError()}
				continue
			}
			results[i] = indexedResult{tc: tc, valid: true, controlErr: errClientToolPending}
			continue
		}

		results[i] = indexedResult{tc: tc, valid: true}
		idx := i

		// g.Go blocks this loop (not a background goroutine) once maxParallel
		// calls are already in flight — the queueing happens here, before a
		// goroutine for this call exists, unlike a semaphore acquired from
		// inside the goroutine body.
		g.Go(func() error {
			// A call still queued when gctx is cancelled must not start real
			// tool work: check before doing anything else so cancellation
			// while queued caps how many tool bodies run, not just how many
			// goroutines are spawned.
			if gctx.Err() != nil {
				err := gctx.Err()
				results[idx].result = &ToolResult{
					ID: tc.id, Name: tc.name, Args: tc.args,
					Output: invalidToolCallOutput(tc, err),
					Error:  classifyToolError(tc.name, err),
				}
				results[idx].modelOutput = results[idx].result.Output
				return nil
			}
			// A panic in a tool executor (or ToModelOutput) is contained to this
			// call: it becomes that tool's error result so sibling tools still
			// complete and the model sees the failure, rather than crashing the
			// process.
			defer safego.Recover(r.logger, func(err error) {
				results[idx].controlErr = err
				if results[idx].result == nil {
					results[idx].result = &ToolResult{
						ID:     tc.id,
						Name:   tc.name,
						Args:   tc.args,
						Output: invalidToolCallOutput(tc, err),
						Error:  classifyToolError(tc.name, err),
					}
				}
				results[idx].modelOutput = results[idx].result.Output
			}, "phase", "parallel-tool-exec", "tool", tc.name)

			var result *ToolResult
			approvalResolved := false
			var approvalErr error
			if skip {
				result = skippedToolResult(tc, skipReason)
			} else {
				result, approvalResolved, approvalErr = approvedToolCall(r, gctx, tools, tc, def, approval, approver)
			}
			if approvalErr != nil {
				results[idx].controlErr = approvalErr
				return nil
			}
			result, hookErr := r.afterTool(result)
			if hookErr != nil {
				results[idx].controlErr = hookErr
				return nil
			}
			results[idx].result = result
			modelOutput, transformErr := transformToolOutput(r, def, result)
			if transformErr != nil {
				result.ModelOutput = result.Output
				result.ModelOutputSet = true
				results[idx].controlErr = transformErr
				results[idx].modelOutput = result.Output
				return nil
			}
			result.ModelOutput = modelOutput
			result.ModelOutputSet = true
			if err := attachApprovalReceipt(r, tc, result, approvalResolved, modelOutput); err != nil {
				results[idx].controlErr = err
				return nil
			}
			results[idx].modelOutput = modelOutput
			return nil
		})
	}
	// g.Go bodies always return nil (per-tool failures ride the results slice,
	// so a sibling failure never cancels the group), so g.Wait blocks until all
	// queued calls finish and its error is expected to be nil. The check is
	// forward-defensive: if a future body ever returns an error, it surfaces
	// instead of being silently dropped.
	if err := g.Wait(); err != nil {
		r.emitError(err)
	}

	// Emit events and build history in original order. A recovered control
	// panic is reported only after the original tool result is visible.
	toolNames = make([]string, 0, len(prepared))
	for _, res := range results {
		if !res.valid {
			if !r.emitObserved(StepEvent{
				Type:              StepEventToolCallInvalid,
				ToolCallID:        res.tc.id,
				ToolCallName:      res.tc.name,
				ToolCallArgsDelta: res.tc.args,
			}) {
				return toolNames, stepToolCalls, stepToolResults, r.stopError()
			}
			if !r.emitObserved(StepEvent{Type: StepEventToolResult, ToolResult: res.result}) {
				return toolNames, stepToolCalls, stepToolResults, r.stopError()
			}
			*history = append(*history, toolResultHistoryMessage(res.result, res.result.ModelOutput))
			toolNames = append(toolNames, res.tc.name)
			stepToolResults = append(stepToolResults, *res.result)
			continue
		}

		if res.result != nil {
			if !r.emitObserved(StepEvent{Type: StepEventToolResult, ToolResult: res.result}) {
				res.controlErr = r.stopError()
			}
			updateLatestToolCallArgs(history, res.result.ID, res.result.Args)
			*history = append(*history, toolResultHistoryMessage(res.result, res.modelOutput))
		}
		toolNames = append(toolNames, res.tc.name)
		approvalID, approvalSignature := "", ""
		if res.result != nil {
			approvalID = res.result.ApprovalID
			approvalSignature = res.result.ApprovalRequestSignature
		}
		stepToolCalls = append(stepToolCalls, ToolCallInfo{
			ID:   res.tc.id,
			Name: res.tc.name,
			Args: func() json.RawMessage {
				if res.result != nil {
					return json.RawMessage(res.result.Args)
				}
				return json.RawMessage(res.tc.args)
			}(),
			ArgsSet:           true,
			ThoughtSignature:  res.tc.thoughtSignature,
			ApprovalID:        approvalID,
			ApprovalSignature: approvalSignature,
		})
		if res.result != nil {
			stepToolResults = append(stepToolResults, *res.result)
		}
		controlErr = preferControlErr(controlErr, res.controlErr)
	}
	return toolNames, stepToolCalls, stepToolResults, controlErr
}

func approvedToolCall(
	r *run,
	ctx context.Context,
	tools *ToolSet,
	tc toolCallState,
	def ToolDefinition,
	approval map[string]func(string, string) bool,
	approver ApprovalResponder,
) (*ToolResult, bool, error) {
	if policy := approval[tc.name]; policy != nil {
		canonicalArgs, _, err := canonicalizeApprovalInput(tc.args)
		if err != nil {
			return nil, false, fmt.Errorf("agent: canonicalize approval input: %w", err)
		}
		tc.args = canonicalArgs
		if _, err := validateToolCall(tools, tc); err != nil {
			return nil, false, fmt.Errorf("agent: validate canonical approval input: %w", err)
		}
		if !policy(tc.name, tc.args) {
			if err := ctx.Err(); err != nil {
				return nil, false, err
			}
			return executeApprovedToolCall(r, ctx, tools, tc, def), false, nil
		}
		approvalID, err := newApprovalID()
		if err != nil {
			return nil, false, err
		}
		request := ApprovalRequest{ApprovalID: approvalID, ToolCallID: tc.id, ToolName: tc.name, Args: tc.args}
		signature, err := signApprovalRequest(r.approvalKey, request)
		if err != nil {
			return nil, false, err
		}
		if !r.emitObserved(
			StepEvent{
				Type:              StepEventToolApprovalRequest,
				ToolCallID:        tc.id,
				ToolCallName:      tc.name,
				ToolCallArgsDelta: tc.args,
				ApprovalID:        request.ApprovalID,
				ApprovalSignature: signature,
			},
		) {
			return nil, false, ctx.Err()
		}
		if approver == nil {
			return nil, false, errApprovalPending
		}
		response, err := approver.RequestApproval(ctx, request)
		if err == nil && response.ApprovalID != request.ApprovalID {
			err = fmt.Errorf(
				"agent: approval responder returned ID %q for request %q",
				response.ApprovalID, request.ApprovalID,
			)
		}
		if err != nil || !response.Approved {
			r.emitObserved(StepEvent{Type: StepEventToolOutputDenied, ToolCallID: tc.id})
			result := deniedToolResult(tc, response.Reason, err)
			result.ApprovalID = request.ApprovalID
			result.ApprovalRequestSignature = signature
			return result, true, nil
		}
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		result := executeApprovedToolCall(r, ctx, tools, tc, def)
		result.ApprovalID = request.ApprovalID
		result.ApprovalRequestSignature = signature
		result.ApprovalApproved = true
		return result, true, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	return executeApprovedToolCall(r, ctx, tools, tc, def), false, nil
}

func executeApprovedToolCall(
	r *run,
	ctx context.Context,
	tools *ToolSet,
	tc toolCallState,
	def ToolDefinition,
) *ToolResult {
	// Built only when tracing is enabled — see the matching comment in
	// runLoop for why this can't just rely on NoopTracer discarding attrs.
	var startAttrs []tracing.Attr
	if r.tracingEnabled {
		startAttrs = []tracing.Attr{{Key: "ai.tool_name", Value: tc.name}}
	}
	toolCtx, span := r.tracer.Start(ctx, "ai.tool_call", startAttrs...)
	defer span.End()
	if r.traceContent {
		span.SetAttributes(tracing.Attr{Key: "ai.tool.arguments", Value: tc.args})
	}
	result := executeToolCallForRun(r, toolCtx, tools, tc, def)
	if r.traceContent {
		span.SetAttributes(tracing.Attr{Key: "ai.tool.output", Value: result.Output})
	}
	return result
}

func transformToolOutput(r *run, def ToolDefinition, result *ToolResult) (output string, err error) {
	output = result.Output
	if def.ToModelOutput == nil {
		return output, nil
	}
	defer safego.Recover(r.logger, func(recovered error) { err = recovered }, "phase", "tool-output-transform")
	output = def.ToModelOutput(result.Output)
	return output, nil
}

func attachApprovalReceipt(
	r *run,
	tc toolCallState,
	result *ToolResult,
	approvalResolved bool,
	modelOutput string,
) error {
	if !approvalResolved {
		return nil
	}
	request := ApprovalRequest{
		ApprovalID: result.ApprovalID, ToolCallID: tc.id, ToolName: tc.name, Args: tc.args,
	}
	receipt, err := signApprovalResult(r.approvalKey, request, result.ApprovalApproved, modelOutput)
	if err != nil {
		return err
	}
	result.ApprovalSignature = receipt
	return nil
}

func toolResultHistoryMessage(result *ToolResult, modelOutput string) Message {
	message := buildToolResultMessageWithApproval(
		result.ID, result.Name, modelOutput, result.ApprovalSignature, result.ApprovalApproved,
	)
	message.Content[0].ToolApprovalID = result.ApprovalID
	if result.Content != nil {
		message.Content[0].ToolResultContent = make([]aikit.ToolResultContent, len(result.Content))
		for i := range result.Content {
			message.Content[0].ToolResultContent[i] = result.Content[i].Clone()
		}
	}
	return message
}

func invalidToolResult(tc toolCallState, err error) *ToolResult {
	output := invalidToolCallOutput(tc, err)
	return &ToolResult{
		ID: tc.id, Name: tc.name, Args: tc.args, Output: output,
		ModelOutput: output, ModelOutputSet: true, Error: err, Disposition: aikit.ToolResultError,
	}
}

func updateLatestToolCallArgs(history *[]Message, toolCallID, args string) {
	for messageIndex := len(*history) - 1; messageIndex >= 0; messageIndex-- {
		message := &(*history)[messageIndex]
		for partIndex := len(message.Content) - 1; partIndex >= 0; partIndex-- {
			part := &message.Content[partIndex]
			if part.Type == "tool_call" && part.ToolCallID == toolCallID {
				part.ToolCallArgs = json.RawMessage(args)
				return
			}
		}
	}
}

func deniedToolResult(
	tc toolCallState,
	reason string,
	cause error,
) *ToolResult {
	return &ToolResult{
		ID:     tc.id,
		Name:   tc.name,
		Args:   tc.args,
		Output: `{"error":"tool approval denied"}`,
		Error: &tool.DeniedError{
			ToolName: tc.name,
			Reason:   reason,
			Cause:    cause,
		},
		Disposition: aikit.ToolResultDenied,
	}
}
