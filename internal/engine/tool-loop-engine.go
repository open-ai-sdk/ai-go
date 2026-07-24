package engine

import (
	"context"
	"fmt"

	"github.com/open-ai-sdk/ai-go/internal/safego"
)

// Run executes the tool loop and streams StepEvents onto the returned channel.
// The channel is closed when the run completes or encounters an unrecoverable error.
func Run(ctx context.Context, params RunParams) <-chan StepEvent {
	ch := make(chan StepEvent, 64)
	go runLoop(ctx, ch, params)
	return ch
}

func runLoop(ctx context.Context, out chan<- StepEvent, params RunParams) {
	r := &run{ctx: ctx, out: out, logger: params.Logger}
	// close(out) is deferred first so it runs last: on a panic in a control
	// callback (PrepareStep, StopWhen, ToModelOutput, RepairToolCall, tool
	// Execute) the error event is emitted before the channel closes, so the
	// consumer sees a *PanicError instead of a cleanly closed empty stream.
	defer close(out)
	defer safego.Recover(r.logger, func(err error) {
		r.emit(StepEvent{Type: StepEventError, Error: err})
	}, "phase", "tool-loop")

	history := buildInitialHistory(params.Request)
	var completedSteps []StepResultInfo
	// lastSR captures the final iteration's streamResult so we can report an
	// accurate finish reason when the loop exits with pending tool_calls at
	// maxSteps (matching ai-sdk-node: honest return, no forced text step).
	var lastSR streamResult

	// MaxSteps <= 0 means unbounded: node has no maxSteps concept at all, so
	// StopWhen (or the model naturally stopping — no tool calls) is the only
	// gate. A caller that sets neither now gets exactly as many steps as
	// StopWhen allows, not an implicit cap.
	for step := 0; params.MaxSteps <= 0 || step < params.MaxSteps; step++ {
		// emit's ctx-guarded send subsumes the old explicit ctx.Err() check: a
		// cancelled context makes the StepStart send return false and unwinds.
		if !r.emit(StepEvent{Type: StepEventStepStart, StepNumber: step}) {
			return
		}

		model := params.Model
		req := params.Request
		req.Instructions = "" // already prepended as system message in history
		req.Messages = history
		if len(req.Tools) == 0 && params.Tools != nil && len(params.Tools.Definitions) > 0 {
			req.Tools = params.Tools.Definitions
		}

		if params.PrepareStep != nil {
			psResult := params.PrepareStep(PrepareStepContext{
				StepNumber:     step,
				Steps:          completedSteps,
				ToolsContext:   req.ToolsContext,
				RuntimeContext: req.RuntimeContext,
			})
			if psResult != nil {
				if psResult.Model != nil {
					model = psResult.Model
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
		}

		// Each step gets a child context so its provider stream can be released
		// deterministically once the step's events are consumed, without waiting
		// for the whole run's context to end.
		stepCtx, cancelStep := context.WithCancel(ctx)
		eventCh, err := model.Stream(stepCtx, req)
		if err != nil {
			cancelStep()
			r.emit(StepEvent{Type: StepEventError, Error: fmt.Errorf("step %d: start stream: %w", step, err)})
			return
		}

		acc := newToolCallAccumulator()
		sr, fatalErr := consumeStream(r, eventCh, acc, params.Callbacks)
		// Release the provider: on the normal path it has already closed; on an
		// early/fatal return, cancelling the child ctx unblocks its ctx-guarded
		// send and the drain lets its goroutine finish and close the body.
		cancelStep()
		if fatalErr {
			go drainStreamEvents(eventCh)
			return
		}
		lastSR = sr
		fullText := sr.text

		if !acc.hasToolCalls() {
			if !r.emit(StepEvent{
				Type:             StepEventStepEnd,
				StepNumber:       step,
				FinishReason:     sr.finish,
				RawFinishReason:  sr.rawFinish,
				ProviderMetadata: sr.providerMeta,
				Warnings:         sr.warnings,
			}) {
				return
			}
			r.safeObserver(func() { emitOnStepEnd(params.Callbacks, step, nil, nil, sr) })
			completedSteps = append(completedSteps, StepResultInfo{
				StepNumber:       step,
				Text:             fullText,
				Reasoning:        sr.reasoning,
				Usage:            sr.usage,
				FinishReason:     sr.finish,
				RawFinishReason:  sr.rawFinish,
				ProviderMetadata: sr.providerMeta,
				Warnings:         sr.warnings,
			})
			emitStructuredOutput(r, params, history)
			r.safeObserver(func() { emitOnEnd(params.Callbacks, completedSteps, sr) })
			r.emit(StepEvent{Type: StepEventDone})
			return
		}

		toolCalls := acc.completed()
		preparedToolCalls := prepareToolCalls(ctx, params.Tools, params.RepairToolCall, req, toolCalls)
		history = append(
			history,
			buildAssistantToolCallMessage(fullText, sr.reasoning, preparedToolCallStates(preparedToolCalls)),
		)

		var toolNames []string
		var stepToolCalls []ToolCallInfo
		var stepToolResults []ToolResult
		if params.ParallelToolExecution {
			toolNames, stepToolCalls, stepToolResults = executeToolCallsParallel(
				r,
				params.Tools,
				preparedToolCalls,
				&history,
				params.MaxParallelTools,
				params.ToolApproval,
				params.Approver,
			)
		} else {
			toolNames, stepToolCalls, stepToolResults = executeToolCalls(
				r, params.Tools, preparedToolCalls, &history, params.ToolApproval, params.Approver,
			)
		}

		if !r.emit(StepEvent{
			Type:             StepEventStepEnd,
			StepNumber:       step,
			FinishReason:     sr.finish,
			RawFinishReason:  sr.rawFinish,
			ProviderMetadata: sr.providerMeta,
			Warnings:         sr.warnings,
		}) {
			return
		}
		r.safeObserver(func() { emitOnStepEnd(params.Callbacks, step, stepToolCalls, stepToolResults, sr) })

		completedSteps = append(completedSteps, StepResultInfo{
			StepNumber:       step,
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
				emitStructuredOutput(r, params, history)
				r.safeObserver(func() { emitOnEnd(params.Callbacks, completedSteps, sr) })
				r.emit(StepEvent{Type: StepEventDone})
				return
			}
		}
	}

	// maxSteps exhausted with pending tool_calls — exit honestly using the
	// last step's streamResult. Caller sees FinishReasonToolCalls and can
	// decide whether to continue (e.g. bump the budget, force tool_choice=none
	// on a follow-up call) or surface the partial result.
	// Historical note: this used to fire a tool-less "final generation" pass
	// which caused gateway Harmony-parsing issues on gpt-oss/gpt-5 family.
	// Matches ai-sdk-node semantics (see packages/ai generate-text.ts:1008).
	emitStructuredOutput(r, params, history)
	r.safeObserver(func() { emitOnEnd(params.Callbacks, completedSteps, lastSR) })
	r.emit(StepEvent{Type: StepEventDone})
}

// drainStreamEvents consumes any remaining events from an abandoned provider
// stream so its decoder goroutine can finish and close the HTTP response body.
func drainStreamEvents(ch <-chan StreamEvent) {
	for range ch {
	}
}
