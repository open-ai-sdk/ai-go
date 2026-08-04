package agent

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/open-ai-sdk/ai-go/internal/tracing"
)

// emitStructuredOutput makes a final constrained LLM call when an OutputSchema
// is configured. It returns false when the run was cancelled or the provider
// failed, in which case the caller must not emit OnEnd or Done.
func emitStructuredOutput(
	r *run,
	params runConfig,
	model Model,
	request Request,
	history []Message,
	completedSteps []StepResultInfo,
) bool {
	if request.Output == nil || request.Output.Type == "text" {
		return true
	}
	step := r.modelCalls
	if err := r.reserveModelCall(); err != nil {
		r.emitError(err)
		return false
	}
	msgs := make([]Message, len(history)+1)
	copy(msgs, history)
	msgs[len(history)] = Message{
		Role:    "user",
		Content: []ContentPart{{Type: "text", Text: "Now produce the structured output as requested."}},
	}

	req := Request{
		Messages:        msgs,
		Output:          request.Output,
		Settings:        request.Settings,
		ProviderOptions: request.ProviderOptions,
		ToolsContext:    request.ToolsContext,
		RuntimeContext:  request.RuntimeContext,
	}
	r.hookContext.Turn = step + 1
	if !r.emitObserved(StepEvent{Type: StepEventStepStart, StepNumber: step}) {
		return false
	}
	applyPrepareStep(params, step, completedSteps, &model, &req)
	var err error
	req, err = r.beforeCompletion(req)
	if err != nil {
		r.emitError(err)
		return false
	}
	// Bind the structured-output call to a child context so it is released when
	// this function returns (including an early return on consumer cancellation).
	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()

	// Built only when tracing is enabled — see the matching comment in
	// runLoop for why this can't just rely on NoopTracer discarding attrs.
	var startAttrs []tracing.Attr
	if r.tracingEnabled {
		startAttrs = []tracing.Attr{
			{Key: "ai.model_id", Value: model.ModelID()},
			{Key: "ai.structured_output", Value: true},
		}
	}
	modelCtx, span := r.tracer.Start(ctx, "ai.model_call", startAttrs...)
	defer span.End()
	if r.traceContent {
		span.SetAttributes(tracing.Attr{Key: "ai.prompt.messages", Value: marshalMessagesForTrace(req.Messages)})
	}

	eventCh, err := model.Stream(modelCtx, req)
	if err != nil {
		span.RecordError(err)
		r.emitError(&StructuredOutputError{
			Kind: StructuredOutputErrorKindPrompt, Reason: "prompt failed", Cause: err,
		})
		return false
	}
	if eventCh == nil {
		err := &StructuredOutputError{
			Kind: StructuredOutputErrorKindEmpty, Reason: "model returned no response",
		}
		span.RecordError(err)
		r.emitError(err)
		return false
	}

	sr, interrupted := consumeStructuredStream(r, eventCh, params.Callbacks, span)
	if interrupted {
		return false
	}
	if r.traceContent {
		span.SetAttributes(tracing.Attr{Key: "ai.completion.text", Value: sr.text})
	}
	if !r.emitObserved(StepEvent{
		Type: StepEventStepEnd, StepNumber: step, MessageID: sr.messageID,
		FinishReason: sr.finish, RawFinishReason: sr.rawFinish,
		ProviderMetadata: sr.providerMeta, Warnings: sr.warnings,
	}) {
		return false
	}
	r.safeObserver(func() { emitOnStepEnd(params.Callbacks, step, nil, nil, sr) })

	raw := sr.text
	if strings.TrimSpace(raw) == "" {
		r.emitError(&StructuredOutputError{
			Kind: StructuredOutputErrorKindEmpty, Path: "$", Reason: "is empty",
		})
		return false
	}
	parsed := parseStructuredOutput(raw)
	if parsed == nil {
		r.emitError(&StructuredOutputError{
			Kind: StructuredOutputErrorKindJSONDecode, Path: "$", Reason: "is invalid JSON",
		})
		return false
	}
	if err := validateStructuredOutput(parsed, request.Output); err != nil {
		r.emitError(err)
		return false
	}
	return r.emitObserved(StepEvent{Type: StepEventStructuredOutput, StructuredOutput: parsed})
}

func consumeStructuredStream(
	r *run,
	events <-chan StreamEvent,
	callbacks *lifecycleCallbacks,
	span tracing.Span,
) (streamResult, bool) {
	var result streamResult
	for {
		select {
		case <-r.ctx.Done():
			return result, true
		case event, ok := <-events:
			if !ok {
				return result, false
			}
			if event.Type == StreamEventError {
				span.RecordError(event.Error)
				r.emitError(&StructuredOutputError{
					Kind:   StructuredOutputErrorKindPrompt,
					Reason: "prompt failed", Cause: event.Error,
				})
				return result, true
			}
			if applyStreamEvent(r, event, &result, nil, callbacks) {
				return result, true
			}
		}
	}
}

// parseStructuredOutput extracts valid JSON from content, stripping markdown fences if present.
func parseStructuredOutput(content string) json.RawMessage {
	content = trimMarkdownFence(content)
	if json.Valid([]byte(content)) {
		return json.RawMessage(content)
	}
	return nil
}

// trimMarkdownFence strips ```json ... ``` or ``` ... ``` fencing.
func trimMarkdownFence(s string) string {
	s = strings.TrimSpace(s)
	for _, prefix := range []string{"```json\n", "```json\r\n", "```\n", "```\r\n"} {
		if strings.HasPrefix(s, prefix) {
			s = s[len(prefix):]
			break
		}
	}
	for _, suffix := range []string{"\n```", "\r\n```", "```"} {
		if strings.HasSuffix(s, suffix) {
			s = s[:len(s)-len(suffix)]
			break
		}
	}
	return strings.TrimSpace(s)
}
