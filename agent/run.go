package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/internal/safego"
	"github.com/open-ai-sdk/ai-go/internal/tracing"
	"github.com/open-ai-sdk/ai-go/transport"
)

// driveStream is the private channel-backed driver used by Runner's iterator.
// The channel closes on completion, cancellation, or terminal failure.
func driveStream(ctx context.Context, params runConfig) <-chan StepEvent {
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
	callbacks *lifecycleCallbacks,
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

//nolint:gocyclo // The driver keeps one explicit state machine and one cleanup boundary.
func runLoop(ctx context.Context, out chan<- StepEvent, params runConfig) error {
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
	// Result.Usage on the canonical aggregated result.
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
		maxTurns: params.MaxSteps, enforceTurns: params.ErrorOnMaxTurns,
		hooks: append([]Hook(nil), params.Hooks...), hookContext: params.HookContext,
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
		r.hookContext.Turn = step + 1
		r.beginTurnBuffer(r.hasModelTurnHooks())
		// emit's ctx-guarded send subsumes the old explicit ctx.Err() check: a
		// cancelled context makes the StepStart send return false and unwinds.
		if !r.emitStreamChunk(StepEvent{Type: StepEventStepStart, StepNumber: step}, params.Callbacks) {
			return r.stopError()
		}

		model := params.Model
		req := cloneRequest(params.Request)
		req.Instructions = "" // already prepended as system message in history
		req.Messages = history
		if req.Tools == nil && params.Tools != nil && params.Tools.Len() > 0 {
			req.Tools = params.Tools.DefinitionsSnapshot()
		}

		applyPrepareStep(params, step, completedSteps, &model, &req)
		var err error
		req, err = r.beforeCompletion(req)
		if err != nil {
			if r.emitError(err) {
				return nil
			}
			return r.stopError()
		}
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

		if err := r.reserveModelCall(); err != nil {
			cancelStep()
			modelSpan.RecordError(err)
			modelSpan.End()
			stepSpan.RecordError(err)
			stepSpan.End()
			if r.emitError(err) {
				return nil
			}
			return r.stopError()
		}
		eventCh, err := model.Stream(modelCtx, req)
		if err != nil {
			// Stream startup failed before there is a channel to consume.
			cancelStep()
			modelSpan.RecordError(err)
			modelSpan.End()
			stepSpan.RecordError(err)
			stepSpan.End()
			if r.emitError(fmt.Errorf("step %d: start stream: %w", step, err)) {
				return nil
			}
			return r.stopError()
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
			r.discardTurnBuffer()
			stepSpan.End()
			return r.stopError()
		}
		lastSR = sr
		fullText := sr.text
		response := CompletionResponseEvent{
			Text: sr.text, Reasoning: sr.reasoning, MessageID: sr.messageID, FinishReason: sr.finish,
		}
		if sr.usage != nil {
			response.Usage = *sr.usage
		}
		if err := r.observeCompletionResponse(response); err != nil {
			r.discardTurnBuffer()
			stepSpan.End()
			if r.emitError(err) {
				return nil
			}
			return r.stopError()
		}
		turnAction, err := r.modelTurn(ModelTurnEvent{
			CompletionResponseEvent: response,
			HasToolCalls:            acc.hasToolCalls(),
		})
		if err != nil {
			r.discardTurnBuffer()
			stepSpan.End()
			if r.emitError(err) {
				return nil
			}
			return r.stopError()
		}
		switch turnAction.Kind {
		case ModelTurnRetry:
			r.discardTurnBuffer()
			stepSpan.End()
			if turnAction.Retry.Feedback != "" {
				rejected := aikit.Message{
					ID: sr.messageID, Role: aikit.RoleAssistant,
					Content: []aikit.ContentPart{aikit.TextPart(sr.text)},
				}
				history = append(history,
					rejected,
					aikit.UserMessage(turnAction.Retry.Feedback),
				)
			}
			continue
		}
		if !r.flushTurnBuffer() {
			stepSpan.End()
			return r.stopError()
		}

		if !acc.hasToolCalls() {
			return finishTextStep(r, params, step, sr, fullText, model, req, history, completedSteps, stepSpan)
		}

		if r.executeToolStep(params, step, sr, fullText, acc, model, req, &history, &completedSteps, stepSpan) {
			return r.stopError()
		}
	}

	if params.ErrorOnMaxTurns {
		if r.emitError(&MaxTurnsError{MaxTurns: params.MaxSteps}) {
			return nil
		}
		return r.stopError()
	}

	// maxSteps exhausted with pending tool_calls — exit honestly using the
	// last step's streamResult. Caller sees FinishReasonToolCalls and can
	// decide whether to continue (e.g. bump the budget, force tool_choice=none
	// on a follow-up call) or surface the partial result.
	// Historical note: this used to fire a tool-less "final generation" pass
	// which caused gateway Harmony-parsing issues on gpt-oss/gpt-5 family.
	if !emitStructuredOutput(r, params, lastModel, lastRequest, history, completedSteps) {
		return r.stopError()
	}
	r.safeObserver(func() { emitOnEnd(params.Callbacks, completedSteps, lastSR) })
	if !r.emitObserved(StepEvent{Type: StepEventDone}) {
		return r.stopError()
	}
	return nil
}

func finishTextStep(
	r *run,
	params runConfig,
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
		return r.stopError()
	}
	r.safeObserver(func() { emitOnStepEnd(params.Callbacks, step, nil, nil, sr) })
	completedSteps = append(completedSteps, StepResultInfo{
		StepNumber: step, MessageID: sr.messageID, Text: fullText, Reasoning: sr.reasoning, Usage: sr.usage,
		FinishReason: sr.finish, RawFinishReason: sr.rawFinish,
		ProviderMetadata: sr.providerMeta, Warnings: sr.warnings,
	})
	if !emitStructuredOutput(r, params, model, req, history, completedSteps) {
		return r.stopError()
	}
	r.safeObserver(func() { emitOnEnd(params.Callbacks, completedSteps, sr) })
	if !r.emitObserved(StepEvent{Type: StepEventDone}) {
		return r.stopError()
	}
	return nil
}

func initializeRunTracing(ctx context.Context, params runConfig) (context.Context, tracing.Tracer, bool) {
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

func prepareRunHistory(r *run, params runConfig) ([]Message, bool, error) {
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
	params runConfig,
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
	// The prepared request is the only source of per-turn tool context.
	r.toolsContext = cloneMap(req.ToolsContext)
	r.runtimeContext = cloneMap(req.RuntimeContext)
	toolCalls := acc.completed()
	preparedToolCalls := prepareToolCalls(r, params.Tools, params.RepairToolCall, req, toolCalls)
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
			if !emitStructuredOutput(r, params, model, req, *history, *completedSteps) {
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
func applyPrepareStep(params runConfig, step int, completedSteps []StepResultInfo, model *Model, req *Request) {
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
