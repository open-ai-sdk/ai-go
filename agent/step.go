package agent

import (
	"context"
	"encoding/json"
	"errors"

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
			r.emit(StepEvent{
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

		r.emit(StepEvent{
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

		result, approvalErr := approvedToolCall(r, r.ctx, tools, tc, preparedCall.def, approval, approver)
		if approvalErr != nil {
			return toolNames, stepToolCalls, stepToolResults, approvalErr
		}
		// The invocation result is caller-visible even if the history-only
		// transform below fails. Emit it before crossing that user callback
		// boundary so a later panic cannot erase completed work.
		r.emit(StepEvent{Type: StepEventToolResult, ToolResult: result})

		// Apply ToModelOutput transform for history; event keeps original output.
		// def was resolved once during validation (prepareToolCalls), so no
		// second scan of tools.Definitions is needed here.
		modelOutput := result.Output
		if preparedCall.def.ToModelOutput != nil {
			modelOutput = preparedCall.def.ToModelOutput(result.Output)
		}

		*history = append(*history, buildToolResultMessage(tc.id, tc.name, modelOutput))
		stepToolResults = append(stepToolResults, *result)
	}
	return toolNames, stepToolCalls, stepToolResults, nil
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
		r.emit(StepEvent{
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

			result, approvalErr := approvedToolCall(r, gctx, tools, tc, def, approval, approver)
			if approvalErr != nil {
				results[idx].controlErr = approvalErr
				return nil
			}
			results[idx].result = result
			modelOutput := result.Output
			if def.ToModelOutput != nil {
				modelOutput = def.ToModelOutput(result.Output)
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
			r.emit(StepEvent{
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
			r.emit(StepEvent{Type: StepEventToolResult, ToolResult: res.result})
			*history = append(*history, buildToolResultMessage(res.tc.id, res.tc.name, res.modelOutput))
		}
		toolNames = append(toolNames, res.tc.name)
		stepToolCalls = append(stepToolCalls, ToolCallInfo{
			ID:               res.tc.id,
			Name:             res.tc.name,
			Args:             json.RawMessage(res.tc.args),
			ArgsSet:          true,
			ThoughtSignature: res.tc.thoughtSignature,
		})
		if res.result != nil {
			stepToolResults = append(stepToolResults, *res.result)
		}
		if controlErr == nil && res.controlErr != nil {
			controlErr = res.controlErr
		}
	}
	if controlErr != nil && !errors.Is(controlErr, errApprovalPending) {
		r.emitError(controlErr)
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
) (*ToolResult, error) {
	if policy := approval[tc.name]; policy != nil && policy(tc.name, tc.args) {
		request := ApprovalRequest{ApprovalID: tc.id, ToolCallID: tc.id, ToolName: tc.name, Args: tc.args}
		r.emit(
			StepEvent{
				Type:              StepEventToolApprovalRequest,
				ToolCallID:        tc.id,
				ToolCallName:      tc.name,
				ToolCallArgsDelta: tc.args,
			},
		)
		if approver == nil {
			return nil, errApprovalPending
		}
		response, err := approver.RequestApproval(ctx, request)
		if err != nil || !response.Approved {
			r.emit(StepEvent{Type: StepEventToolOutputDenied, ToolCallID: tc.id})
			return deniedToolResult(tc, response.Reason, err), nil
		}
	}

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
	return result, nil
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
