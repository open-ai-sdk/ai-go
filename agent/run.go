// ai-go: file-length-justification: keeps tool-loop lifecycle, cancellation, tracing, and terminal-event ownership together.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/open-ai-sdk/ai-go/internal/safego"
	"github.com/open-ai-sdk/ai-go/internal/tracing"
	"github.com/open-ai-sdk/ai-go/transport"
)

// Stream executes the tool loop and streams StepEvents onto the returned
// channel. The channel is closed when the run completes, the context is
// cancelled, or an unrecoverable error occurs.
func Stream(ctx context.Context, params RunParams) <-chan StepEvent {
	ch := make(chan StepEvent, 64)
	go func() {
		// This is the package's outer ownership boundary. It deliberately wraps
		// tracer/logger initialization and every deferred cleanup in runLoop so a
		// panic anywhere in the goroutine still reports an error and closes ch.
		defer close(ch)
		defer safego.Recover(params.Logger, func(err error) {
			emitTerminalError(ch, err, params.Logger, params.Callbacks)
		}, "phase", "tool-loop")
		if err := runLoop(ctx, ch, params); err != nil {
			emitTerminalError(ch, err, params.Logger, params.Callbacks)
		}
	}()
	return ch
}

// emitTerminalError guarantees an active consumer can observe why a run
// stopped while still guaranteeing prompt close for an abandoned consumer.
// If cancellation found the bounded buffer full, one already-truncated event
// is discarded to reserve the terminal error slot.
func emitTerminalError(
	ch chan StepEvent,
	err error,
	logger *slog.Logger,
	callbacks *LifecycleCallbacks,
) {
	event := StepEvent{Type: StepEventError, Error: err}
	for {
		select {
		case ch <- event:
			if callbacks != nil && callbacks.OnChunk != nil {
				callbackEvent := snapshotStepEvent(event)
				safeObserver(logger, func() { callbacks.OnChunk(callbackEvent) })
			}
			notifyError(logger, callbacks, err)
			return
		default:
		}
		select {
		case <-ch:
		default:
		}
	}
}

// Run executes the tool loop and returns its event stream. It is equivalent
// to [Stream]; both names are kept because callers commonly describe the
// blocking aggregation path as a run and the live path as a stream.
func Run(ctx context.Context, params RunParams) <-chan StepEvent {
	return Stream(ctx, params)
}

func runLoop(ctx context.Context, out chan<- StepEvent, params RunParams) error {
	ctx, tracer, tracingEnabled := initializeRunTracing(ctx, params)
	ctx, runSpan := tracer.Start(ctx, "ai.run")

	var completedSteps []StepResultInfo
	// lastSR captures the final iteration's streamResult so we can report an
	// accurate finish reason when the loop exits with pending tool_calls at
	// maxSteps, and
	// so the run span reports the run's terminal state (see the deferred
	// closure below).
	var lastSR streamResult
	lastModel := params.Model
	lastRequest := params.Request
	// Reports the run's terminal finish reason/usage on the span regardless of
	// which of the loop's several exit points was taken. This mirrors the
	// last completed step's outcome rather than a lifetime sum across steps —
	// the cumulative total remains available to the caller via
	// GenerateTextResult.Usage in the ai package.
	defer func() {
		if tracingEnabled {
			runSpan.SetAttributes(stepAttrs(lastSR)...)
		}
		runSpan.End()
	}()

	r := &run{
		ctx: ctx, out: out, logger: params.Logger, callbacks: params.Callbacks,
		approvalKey:         append([]byte(nil), params.ApprovalKey...),
		approvalReplayGuard: params.ApprovalReplayGuard,
		tracer:              tracer, tracingEnabled: tracingEnabled, traceContent: params.TraceContent,
	}
	history, proceed, err := prepareRunHistory(r, params)
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}

	// MaxSteps <= 0 means unbounded, so StopWhen (or the model naturally
	// stopping — no tool calls) is the only
	// gate. A caller that sets neither now gets exactly as many steps as
	// StopWhen allows, not an implicit cap.
	for step := 0; params.MaxSteps <= 0 || step < params.MaxSteps; step++ {
		// emit's ctx-guarded send subsumes the old explicit ctx.Err() check: a
		// cancelled context makes the StepStart send return false and unwinds.
		if !r.emitObserved(StepEvent{Type: StepEventStepStart, StepNumber: step}) {
			return ctx.Err()
		}

		model := params.Model
		req := params.Request
		req.Instructions = "" // already prepended as system message in history
		req.Messages = history
		if req.Tools == nil && params.Tools != nil && len(params.Tools.Definitions) > 0 {
			req.Tools = params.Tools.Definitions
		}

		applyPrepareStep(params, step, completedSteps, &model, &req)
		lastModel = model
		lastRequest = req

		// Each step gets a child context so its provider stream can be released
		// deterministically once the step's events are consumed, without waiting
		// for the whole run's context to end.
		stepCtx, cancelStep := context.WithCancel(ctx)
		// Built only when tracing is enabled, and shared by both spans started
		// below: passing attrs through the Tracer interface allocates a backing
		// array unconditionally (escape analysis can't see past the dynamic
		// dispatch to know NoopTracer discards it), so skipping the literal
		// here — not just relying on NoopTracer's no-op body — is what keeps
		// the disabled path allocation-free.
		var spanAttrs []tracing.Attr
		if tracingEnabled {
			spanAttrs = []tracing.Attr{
				{Key: "ai.step_number", Value: step},
				{Key: "ai.model_id", Value: model.ModelID()},
			}
		}
		stepCtx, stepSpan := tracer.Start(stepCtx, "ai.step", spanAttrs...)

		// The model call gets its own nested span. Started and ended inline here
		// (not factored into a helper function returning a struct) because
		// moving this same logic behind a function boundary was measured to add
		// two heap allocations per step even with tracing disabled — the Go
		// compiler could no longer prove the span value and its attributes
		// stayed off the heap once they crossed a return. Keeping it inline
		// keeps the disabled path's allocation count within one of the
		// pre-instrumentation baseline.
		modelCtx, modelSpan := tracer.Start(stepCtx, "ai.model_call", spanAttrs...)
		if r.traceContent {
			modelSpan.SetAttributes(
				tracing.Attr{Key: "ai.prompt.messages", Value: marshalMessagesForTrace(req.Messages)},
			)
		}

		eventCh, err := model.Stream(modelCtx, req)
		if err != nil {
			// Stream startup failed before there is a channel to consume.
			cancelStep()
			modelSpan.RecordError(err)
			modelSpan.End()
			stepSpan.RecordError(err)
			stepSpan.End()
			r.emitError(fmt.Errorf("step %d: start stream: %w", step, err))
			return ctx.Err()
		}

		acc := newToolCallAccumulator()
		sr, interrupted := consumeStream(r, eventCh, acc, params.Callbacks)
		// Release the provider only after its normal stream has closed. On a
		// fatal event consumeStream returns early, so cancellation unblocks any
		// remaining ctx-guarded sends before the drain below.
		cancelStep()
		if tracingEnabled {
			modelSpan.SetAttributes(stepAttrs(sr)...)
		}
		if r.traceContent {
			modelSpan.SetAttributes(tracing.Attr{Key: "ai.completion.text", Value: sr.text})
		}
		modelSpan.End()
		if interrupted {
			stepSpan.End()
			return ctx.Err()
		}
		lastSR = sr
		fullText := sr.text

		if !acc.hasToolCalls() {
			return finishTextStep(r, params, step, sr, fullText, model, req, history, completedSteps, stepSpan)
		}

		if r.executeToolStep(params, step, sr, fullText, acc, model, req, &history, &completedSteps, stepSpan) {
			return ctx.Err()
		}
	}

	// maxSteps exhausted with pending tool_calls — exit honestly using the
	// last step's streamResult. Caller sees FinishReasonToolCalls and can
	// decide whether to continue (e.g. bump the budget, force tool_choice=none
	// on a follow-up call) or surface the partial result.
	// Historical note: this used to fire a tool-less "final generation" pass
	// which caused gateway Harmony-parsing issues on gpt-oss/gpt-5 family.
	if !emitStructuredOutput(r, lastModel, lastRequest, history) {
		return ctx.Err()
	}
	r.safeObserver(func() { emitOnEnd(params.Callbacks, completedSteps, lastSR) })
	r.emitObserved(StepEvent{Type: StepEventDone})
	return nil
}

func finishTextStep(
	r *run,
	params RunParams,
	step int,
	sr streamResult,
	fullText string,
	model Model,
	req Request,
	history []Message,
	completedSteps []StepResultInfo,
	stepSpan tracing.Span,
) error {
	if r.tracingEnabled {
		stepSpan.SetAttributes(stepAttrs(sr)...)
	}
	stepSpan.End()
	if !r.emitObserved(StepEvent{
		Type: StepEventStepEnd, StepNumber: step, MessageID: sr.messageID, FinishReason: sr.finish,
		RawFinishReason: sr.rawFinish, ProviderMetadata: sr.providerMeta, Warnings: sr.warnings,
	}) {
		return r.ctx.Err()
	}
	r.safeObserver(func() { emitOnStepEnd(params.Callbacks, step, nil, nil, sr) })
	completedSteps = append(completedSteps, StepResultInfo{
		StepNumber: step, MessageID: sr.messageID, Text: fullText, Reasoning: sr.reasoning, Usage: sr.usage,
		FinishReason: sr.finish, RawFinishReason: sr.rawFinish,
		ProviderMetadata: sr.providerMeta, Warnings: sr.warnings,
	})
	if !emitStructuredOutput(r, model, req, history) {
		return r.ctx.Err()
	}
	r.safeObserver(func() { emitOnEnd(params.Callbacks, completedSteps, sr) })
	r.emitObserved(StepEvent{Type: StepEventDone})
	return nil
}

func initializeRunTracing(ctx context.Context, params RunParams) (context.Context, tracing.Tracer, bool) {
	tracer := params.Tracer
	enabled := tracer != nil
	if !enabled {
		tracer = tracing.NoopTracer{}
	}
	if params.Logger != nil {
		ctx = transport.WithLogger(ctx, params.Logger)
	}
	return ctx, tracer, enabled
}

func prepareRunHistory(r *run, params RunParams) ([]Message, bool, error) {
	if err := params.Tools.Validate(); err != nil {
		r.emitError(err)
		return nil, false, nil
	}
	history, err := resumeToolApprovals(r, params, buildInitialHistory(params.Request))
	if err == nil {
		return history, true, nil
	}
	if r.ctx.Err() != nil {
		return nil, false, r.ctx.Err()
	}
	r.emitError(err)
	return nil, false, nil
}

// executeToolStep runs a step's tool calls, ends the step span, emits the
// step-end event, records the completed step, and evaluates StopWhen. It returns
// true when the run should stop — a control emit failed (consumer gone), or
// StopWhen fired (in which case it has already emitted the terminal Done event).
func (r *run) executeToolStep(
	params RunParams,
	step int,
	sr streamResult,
	fullText string,
	acc *toolCallAccumulator,
	model Model,
	req Request,
	history *[]Message,
	completedSteps *[]StepResultInfo,
	stepSpan tracing.Span,
) bool {
	toolCalls := acc.completed()
	preparedToolCalls := prepareToolCalls(r.ctx, params.Tools, params.RepairToolCall, req, toolCalls)
	*history = append(
		*history,
		buildAssistantToolCallMessage(sr.messageID, fullText, sr.reasoning, preparedToolCallStates(preparedToolCalls)),
	)

	var toolNames []string
	var stepToolCalls []ToolCallInfo
	var stepToolResults []ToolResult
	var controlErr error
	if params.ParallelToolExecution {
		toolNames, stepToolCalls, stepToolResults, controlErr = executeToolCallsParallel(
			r, params.Tools, preparedToolCalls, history,
			params.MaxParallelTools, params.ToolApproval, params.Approver,
		)
	} else {
		toolNames, stepToolCalls, stepToolResults, controlErr = executeToolCalls(
			r, params.Tools, preparedToolCalls, history, params.ToolApproval, params.Approver,
		)
	}

	if controlErr != nil {
		if !errors.Is(controlErr, errApprovalPending) {
			stepSpan.RecordError(controlErr)
		}
		stepSpan.End()
		if errors.Is(controlErr, errApprovalPending) {
			r.emitObserved(StepEvent{
				Type:         StepEventStepEnd,
				StepNumber:   step,
				MessageID:    sr.messageID,
				FinishReason: FinishReasonToolCalls,
			})
			r.safeObserver(func() { emitOnStepEnd(params.Callbacks, step, stepToolCalls, stepToolResults, sr) })
			*completedSteps = append(*completedSteps, StepResultInfo{
				StepNumber:   step,
				MessageID:    sr.messageID,
				HasToolCalls: true,
				ToolNames:    toolNames,
				Text:         fullText,
				Reasoning:    sr.reasoning,
				ToolCalls:    stepToolCalls,
				ToolResults:  stepToolResults,
				Usage:        sr.usage,
				FinishReason: FinishReasonToolCalls,
			})
			r.safeObserver(func() { emitOnEnd(params.Callbacks, *completedSteps, sr) })
			r.emitObserved(StepEvent{Type: StepEventDone})
		} else {
			r.emitError(controlErr)
		}
		return true
	}

	if r.tracingEnabled {
		stepSpan.SetAttributes(stepAttrs(sr)...)
	}
	stepSpan.End()

	if !r.emitObserved(StepEvent{
		Type:             StepEventStepEnd,
		StepNumber:       step,
		MessageID:        sr.messageID,
		FinishReason:     sr.finish,
		RawFinishReason:  sr.rawFinish,
		ProviderMetadata: sr.providerMeta,
		Warnings:         sr.warnings,
	}) {
		return true
	}
	r.safeObserver(func() { emitOnStepEnd(params.Callbacks, step, stepToolCalls, stepToolResults, sr) })

	*completedSteps = append(*completedSteps, StepResultInfo{
		StepNumber:       step,
		MessageID:        sr.messageID,
		HasToolCalls:     true,
		ToolNames:        toolNames,
		Text:             fullText,
		Reasoning:        sr.reasoning,
		ToolCalls:        stepToolCalls,
		ToolResults:      stepToolResults,
		Usage:            sr.usage,
		FinishReason:     sr.finish,
		RawFinishReason:  sr.rawFinish,
		ProviderMetadata: sr.providerMeta,
		Warnings:         sr.warnings,
	})

	if params.StopWhen != nil {
		stopResult := &StepResult{HasToolCalls: true, ToolNames: toolNames, Text: fullText}
		if params.StopWhen(step+1, stopResult) {
			if !emitStructuredOutput(r, model, req, *history) {
				return true
			}
			r.safeObserver(func() { emitOnEnd(params.Callbacks, *completedSteps, sr) })
			r.emitObserved(StepEvent{Type: StepEventDone})
			return true
		}
	}
	return false
}

// applyPrepareStep runs the PrepareStep callback (if configured) and applies its
// non-nil overrides to the current step's model and request in place.
func applyPrepareStep(params RunParams, step int, completedSteps []StepResultInfo, model *Model, req *Request) {
	if params.PrepareStep == nil {
		return
	}
	psResult := params.PrepareStep(PrepareStepContext{
		StepNumber:     step,
		Steps:          snapshotPrepareStepInfos(completedSteps),
		ToolsContext:   req.ToolsContext,
		RuntimeContext: req.RuntimeContext,
	})
	if psResult == nil {
		return
	}
	if psResult.Model != nil {
		*model = psResult.Model
	}
	if psResult.ToolChoice != nil {
		req.ToolChoice = psResult.ToolChoice
	}
	if psResult.Instructions != "" {
		req.Instructions = psResult.Instructions
	}
	if psResult.ProviderOptions != nil {
		req.ProviderOptions = mergeProviderOptions(req.ProviderOptions, psResult.ProviderOptions)
	}
	if psResult.ActiveTools != nil {
		req.Tools = filterTools(req.Tools, psResult.ActiveTools)
	}
}
