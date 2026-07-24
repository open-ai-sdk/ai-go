package engine

import (
	"context"

	"github.com/open-ai-sdk/ai-go/internal/tracing"
)

// modelCallResult carries everything runLoop needs after one model call: the
// accumulated stream result, the tool-call accumulator, whether the stream
// ended in a fatal error, an error from starting the stream itself (distinct
// from a fatal mid-stream error, which surfaces through sr/fatal instead),
// and the raw event channel so a fatal exit can drain it.
type modelCallResult struct {
	sr      streamResult
	acc     *toolCallAccumulator
	fatal   bool
	err     error
	eventCh <-chan StreamEvent
}

// runStepModelCall wraps one step's model.Stream call and the consumption of
// its events in an "ai.model_call" span. It is a standalone function, not
// inlined in runLoop's loop body, specifically so span.End can be deferred
// normally: a defer written directly inside a for loop only runs at function
// exit, not at the end of that iteration, which would leak one open span per
// step until the whole run finished.
func runStepModelCall(
	r *run,
	tracer tracing.Tracer,
	ctx context.Context,
	model Model,
	req Request,
	step int,
	cb *LifecycleCallbacks,
) modelCallResult {
	modelCtx, span := tracer.Start(ctx, "ai.model_call",
		tracing.Attr{Key: "ai.step_number", Value: step},
		tracing.Attr{Key: "ai.model_id", Value: model.ModelID()},
	)
	defer span.End()
	if r.traceContent {
		span.SetAttributes(tracing.Attr{Key: "ai.prompt.messages", Value: marshalMessagesForTrace(req.Messages)})
	}

	eventCh, err := model.Stream(modelCtx, req)
	if err != nil {
		span.RecordError(err)
		return modelCallResult{err: err}
	}

	acc := newToolCallAccumulator()
	sr, fatal := consumeStream(r, eventCh, acc, cb)
	span.SetAttributes(stepAttrs(sr)...)
	if r.traceContent {
		span.SetAttributes(tracing.Attr{Key: "ai.completion.text", Value: sr.text})
	}
	return modelCallResult{sr: sr, acc: acc, fatal: fatal, eventCh: eventCh}
}
