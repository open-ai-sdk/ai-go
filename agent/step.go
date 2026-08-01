package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
		if preparedCall.invalidErr != nil {
			r.emitObserved(StepEvent{
				Type:              StepEventToolCallInvalid,
				ToolCallID:        tc.id,
				ToolCallName:      tc.name,
				ToolCallArgsDelta: tc.args,
			})
			errOutput := invalidToolCallOutput(tc, preparedCall.invalidErr)
			*history = append(*history, buildToolResultMessage(tc.id, tc.name, errOutput))
			toolNames = append(toolNames, tc.name)
			continue
		}

		r.emitObserved(StepEvent{
			Type:              StepEventToolCallReady,
			ToolCallID:        tc.id,
			ToolCallName:      tc.name,
			ToolCallArgsDelta: tc.args,
			ThoughtSignature:  tc.thoughtSignature,
		})
		toolNames = append(toolNames, tc.name)
		stepToolCalls = append(stepToolCalls, ToolCallInfo{
			ID:               tc.id,
			Name:             tc.name,
			Args:             json.RawMessage(tc.args),
			ArgsSet:          true,
			ThoughtSignature: tc.thoughtSignature,
		})

		result, approvalResolved, approvalErr := approvedToolCall(r, r.ctx, tools, tc, preparedCall.def, approval, approver)
		if approvalErr != nil {
			if controlErr == nil {
				controlErr = approvalErr
			}
			continue
		}
		// Apply ToModelOutput transform for history; event keeps original output.
		// def was resolved once during validation (prepareToolCalls), so no
		// second scan of tools.Definitions is needed here.
		modelOutput, transformErr := transformToolOutput(r, preparedCall.def, result)
		if transformErr != nil {
			result.ModelOutput = result.Output
			result.ModelOutputSet = true
			r.emitObserved(StepEvent{Type: StepEventToolResult, ToolResult: result})
			if controlErr == nil {
				controlErr = transformErr
			}
			continue
		}
		result.ModelOutput = modelOutput
		result.ModelOutputSet = true
		if err := attachApprovalReceipt(r, tc, result, approvalResolved, modelOutput); err != nil {
			if controlErr == nil {
				controlErr = err
			}
			continue
		}
		if result.ApprovalID != "" {
			stepToolCalls[len(stepToolCalls)-1].ApprovalID = result.ApprovalID
			stepToolCalls[len(stepToolCalls)-1].ApprovalSignature = result.ApprovalRequestSignature
		}
		stepToolCalls[len(stepToolCalls)-1].Args = json.RawMessage(result.Args)

		r.emitObserved(StepEvent{Type: StepEventToolResult, ToolResult: result})
		updateLatestToolCallArgs(history, result.ID, result.Args)
		*history = append(*history, toolResultHistoryMessage(result, modelOutput))
		stepToolResults = append(stepToolResults, *result)
	}
	return toolNames, stepToolCalls, stepToolResults, controlErr
}

// executeToolCallsParallel processes tool calls concurrently, bounded by
// maxParallel via errgroup.SetLimit. g.Go always returns nil: node continues
// sibling tool calls when one fails and reports the failure per-call, so an
// errgroup first-error-cancels policy would silently change that semantic.
// Per-call errors travel through results[i].result.Output instead of through
// g.Wait's return value. errgroup is used here only for concurrency limiting
// and ctx propagation, not for fail-fast error aggregation.
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
		if preparedCall.invalidErr != nil {
			results[i] = indexedResult{
				tc: tc, valid: false,
				result: &ToolResult{
					ID: tc.id, Name: tc.name, Args: tc.args,
					Output: invalidToolCallOutput(tc, preparedCall.invalidErr),
				},
			}
			continue
		}

		// Emit ToolCallReady before execution starts (matches sequential contract).
		r.emitObserved(StepEvent{
			Type:              StepEventToolCallReady,
			ToolCallID:        tc.id,
			ToolCallName:      tc.name,
			ToolCallArgsDelta: tc.args,
			ThoughtSignature:  tc.thoughtSignature,
		})

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

			result, approvalResolved, approvalErr := approvedToolCall(r, gctx, tools, tc, def, approval, approver)
			if approvalErr != nil {
				results[idx].controlErr = approvalErr
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
			r.emitObserved(StepEvent{
				Type:              StepEventToolCallInvalid,
				ToolCallID:        res.tc.id,
				ToolCallName:      res.tc.name,
				ToolCallArgsDelta: res.tc.args,
			})
			*history = append(*history, buildToolResultMessage(res.tc.id, res.tc.name, res.result.Output))
			toolNames = append(toolNames, res.tc.name)
			continue
		}

		if res.result != nil {
			r.emitObserved(StepEvent{Type: StepEventToolResult, ToolResult: res.result})
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
		if res.controlErr != nil && (controlErr == nil ||
			(errors.Is(controlErr, errApprovalPending) && !errors.Is(res.controlErr, errApprovalPending))) {
			controlErr = res.controlErr
		}
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
	result := executeToolCall(toolCtx, tools, tc, def)
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
	return message
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
	}
}
