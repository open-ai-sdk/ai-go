package agent

import "encoding/json"

// streamResult holds accumulated metadata from consuming a model stream.
type streamResult struct {
	text         string
	reasoning    string
	messageID    string
	finish       FinishReason
	rawFinish    string
	providerMeta map[string]any
	warnings     []Warning
	usage        *Usage
}

// consumeStream reads all events from a model stream, forwards them to the step
// event channel, and accumulates tool calls via acc (may be nil for text-only calls).
// Returns the accumulated result and true if a fatal error was emitted.
func consumeStream(
	r *run,
	eventCh <-chan StreamEvent,
	acc *toolCallAccumulator,
	cb *lifecycleCallbacks,
) (streamResult, bool) {
	var sr streamResult
	for {
		select {
		case <-r.ctx.Done():
			return sr, true
		case ev, ok := <-eventCh:
			if !ok {
				return sr, false
			}
			if interrupted := applyStreamEvent(r, ev, &sr, acc, cb); interrupted {
				return sr, true
			}
		}
	}
}

// applyStreamEvent dispatches a single StreamEvent: updates sr in-place,
// forwards StepEvents to out, and fires lifecycle callbacks. Returns true
// if the event was a fatal StreamEventError.
func applyStreamEvent(
	r *run,
	ev StreamEvent,
	sr *streamResult,
	acc *toolCallAccumulator,
	cb *lifecycleCallbacks,
) bool {
	emitChunk := func(stepEv StepEvent) bool {
		return r.emitStreamChunk(stepEv, cb)
	}
	switch ev.Type {
	case StreamEventTextDelta:
		sr.text += ev.TextDelta
		if err := r.observeTextDelta(TextDeltaEvent{Delta: ev.TextDelta, Text: sr.text}); err != nil {
			r.setHookError(err)
			return true
		}
		return !emitChunk(StepEvent{Type: StepEventTextDelta, TextDelta: ev.TextDelta})

	case StreamEventReasoningDelta:
		sr.reasoning += ev.TextDelta
		return !emitChunk(StepEvent{
			Type:             StepEventReasoningDelta,
			ReasoningDelta:   ev.TextDelta,
			ThoughtSignature: ev.ThoughtSignature,
		})

	case StreamEventToolCallDelta:
		return !handleToolCallDelta(r, ev, acc, cb)

	case StreamEventUsage:
		// Providers may report usage across several events within one step
		// (e.g. Anthropic emits input/cache tokens up front and the final
		// output count later). Merge non-zero fields so no partial update
		// clobbers a previously reported count.
		sr.usage = mergeUsage(sr.usage, ev.Usage)
		return !emitChunk(StepEvent{Type: StepEventUsage, Usage: sr.usage})

	case StreamEventSource:
		if ev.Source != nil {
			return !emitChunk(StepEvent{Type: StepEventSource, Source: ev.Source})
		}

	case StreamEventFileDelta:
		return !emitChunk(StepEvent{
			Type:          StepEventFileDelta,
			FileData:      ev.FileData,
			FileMediaType: ev.FileMediaType,
		})

	case StreamEventFinish:
		sr.messageID = ev.MessageID
		sr.finish = ev.FinishReason
		sr.rawFinish = ev.RawFinishReason
		sr.providerMeta = ev.ProviderMetadata
		if len(ev.Warnings) > 0 {
			sr.warnings = append(sr.warnings, ev.Warnings...)
		}
		response := CompletionResponseEvent{
			Text: sr.text, Reasoning: sr.reasoning, MessageID: sr.messageID, FinishReason: sr.finish,
		}
		if sr.usage != nil {
			response.Usage = *sr.usage
		}
		if err := r.observeStreamFinish(StreamFinishEvent{CompletionResponseEvent: response}); err != nil {
			r.setHookError(err)
			return true
		}

	case StreamEventError:
		r.emitError(ev.Error)
		return true
	}
	return false
}

// handleToolCallDelta handles a StreamEventToolCallDelta event by forwarding
// either a tool-call-start or a tool-call-delta StepEvent to out and the
// optional chunk callback. It is a no-op when acc is nil.
func handleToolCallDelta(
	r *run,
	ev StreamEvent,
	acc *toolCallAccumulator,
	cb *lifecycleCallbacks,
) bool {
	if acc == nil {
		return true
	}
	isNew := acc.add(ev)
	if isNew {
		stepEv := StepEvent{
			Type:              StepEventToolCallStart,
			ToolCallIndex:     ev.ToolCallIndex,
			ToolCallID:        ev.ToolCallID,
			ToolCallName:      ev.ToolCallName,
			ToolCallArgsDelta: ev.ToolCallArgsDelta,
			ThoughtSignature:  ev.ThoughtSignature,
		}
		if err := r.observeToolCallDelta(ToolCallDeltaEvent{
			ID: ev.ToolCallID, Name: ev.ToolCallName, Index: ev.ToolCallIndex,
			Delta: json.RawMessage(ev.ToolCallArgsDelta),
		}); err != nil {
			r.setHookError(err)
			return false
		}
		if !r.emitStreamChunk(stepEv, cb) {
			return false
		}
	} else if ev.ToolCallArgsDelta != "" {
		stepEv := StepEvent{
			Type:              StepEventToolCallDelta,
			ToolCallIndex:     ev.ToolCallIndex,
			ToolCallID:        ev.ToolCallID,
			ToolCallArgsDelta: ev.ToolCallArgsDelta,
		}
		if err := r.observeToolCallDelta(ToolCallDeltaEvent{
			ID: ev.ToolCallID, Index: ev.ToolCallIndex, Delta: json.RawMessage(ev.ToolCallArgsDelta),
		}); err != nil {
			r.setHookError(err)
			return false
		}
		if !r.emitStreamChunk(stepEv, cb) {
			return false
		}
	}
	return true
}

// mergeUsage combines partial usage reports from the same step without
// discarding fields populated by an earlier provider event. The merge strategy
// lives in aikit.Usage.Merge; this wrapper owns only the agent's nil policy,
// where an absent report leaves the prior snapshot untouched.
func mergeUsage(prior, incoming *Usage) *Usage {
	if incoming == nil {
		return snapshotUsage(prior)
	}
	if prior == nil {
		return snapshotUsage(incoming)
	}
	merged := prior.Merge(*incoming)
	return &merged
}

// executeToolCalls processes a batch of completed tool calls: validates JSON args,
// emits StepEventToolCallInvalid for invalid args (with error result for model retry),
// then fires events and runs executors for valid calls.
