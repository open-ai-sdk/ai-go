package agent

// streamResult holds accumulated metadata from consuming a model stream.
type streamResult struct {
	text         string
	reasoning    string
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
	cb *LifecycleCallbacks,
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
	cb *LifecycleCallbacks,
) bool {
	emitChunk := func(stepEv StepEvent) bool {
		if !r.emit(stepEv) {
			return false
		}
		if cb != nil && cb.OnChunk != nil {
			callbackEvent := snapshotStepEvent(stepEv)
			r.safeObserver(func() { cb.OnChunk(callbackEvent) })
		}
		return true
	}
	switch ev.Type {
	case StreamEventTextDelta:
		sr.text += ev.TextDelta
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
		sr.finish = ev.FinishReason
		sr.rawFinish = ev.RawFinishReason
		sr.providerMeta = ev.ProviderMetadata
		if len(ev.Warnings) > 0 {
			sr.warnings = append(sr.warnings, ev.Warnings...)
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
	cb *LifecycleCallbacks,
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
		if !r.emit(stepEv) {
			return false
		}
		if cb != nil && cb.OnChunk != nil {
			r.safeObserver(func() { cb.OnChunk(stepEv) })
		}
	} else if ev.ToolCallArgsDelta != "" {
		stepEv := StepEvent{
			Type:              StepEventToolCallDelta,
			ToolCallIndex:     ev.ToolCallIndex,
			ToolCallID:        ev.ToolCallID,
			ToolCallArgsDelta: ev.ToolCallArgsDelta,
		}
		if !r.emit(stepEv) {
			return false
		}
		if cb != nil && cb.OnChunk != nil {
			r.safeObserver(func() { cb.OnChunk(stepEv) })
		}
	}
	return true
}

// mergeUsage combines partial usage reports from the same step without
// discarding fields populated by an earlier provider event.
func mergeUsage(prior, incoming *Usage) *Usage {
	if incoming == nil {
		return snapshotUsage(prior)
	}
	if prior == nil {
		return snapshotUsage(incoming)
	}
	takeInt := func(current, next int) int {
		if next != 0 {
			return next
		}
		return current
	}
	merged := *prior
	merged.InputTokens = takeInt(prior.InputTokens, incoming.InputTokens)
	merged.OutputTokens = takeInt(prior.OutputTokens, incoming.OutputTokens)
	merged.TotalTokens = takeInt(prior.TotalTokens, incoming.TotalTokens)
	merged.InputTokenDetails.NoCacheTokens = takeInt(
		prior.InputTokenDetails.NoCacheTokens,
		incoming.InputTokenDetails.NoCacheTokens,
	)
	merged.InputTokenDetails.CacheReadTokens = takeInt(
		prior.InputTokenDetails.CacheReadTokens,
		incoming.InputTokenDetails.CacheReadTokens,
	)
	merged.InputTokenDetails.CacheWriteTokens = takeInt(
		prior.InputTokenDetails.CacheWriteTokens,
		incoming.InputTokenDetails.CacheWriteTokens,
	)
	merged.OutputTokenDetails.TextTokens = takeInt(
		prior.OutputTokenDetails.TextTokens,
		incoming.OutputTokenDetails.TextTokens,
	)
	merged.OutputTokenDetails.ReasoningTokens = takeInt(
		prior.OutputTokenDetails.ReasoningTokens,
		incoming.OutputTokenDetails.ReasoningTokens,
	)
	if incoming.Raw != nil {
		merged.Raw = snapshotJSONMap(incoming.Raw)
	} else {
		merged.Raw = snapshotJSONMap(prior.Raw)
	}
	return &merged
}

// executeToolCalls processes a batch of completed tool calls: validates JSON args,
// emits StepEventToolCallInvalid for invalid args (with error result for model retry),
// then fires events and runs executors for valid calls.
