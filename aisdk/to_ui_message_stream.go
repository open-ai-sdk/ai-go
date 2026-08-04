package aisdk

import (
	"iter"
	"sync"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// ToUIStreamOptions configures the ToUIMessageStream bridge.
type ToUIStreamOptions struct {
	// SendReasoning forwards reasoning-start/delta/end chunks when true.
	SendReasoning bool
	// SendSources forwards source chunks when true.
	SendSources bool
	// MessageMetadata is called to attach metadata to the finish chunk.
	// If nil, no metadata is attached.
	MessageMetadata func(info MessageMetadataInfo) map[string]any

	// SendStart controls whether the start chunk is emitted (default: nil = true).
	// Set to boolPtr(false) when merging into an outer stream that already emitted start.
	SendStart *bool
	// SendFinish controls whether the finish chunk is emitted (default: nil = true).
	// Set to boolPtr(false) when the outer stream manages the finish lifecycle.
	SendFinish *bool
}

// boolVal returns the dereferenced value of a *bool, defaulting to def if nil.
func boolVal(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}

// MessageMetadataInfo provides context for the MessageMetadata callback.
type MessageMetadataInfo struct {
	FinishReason string
	Usage        *UsageInfo
}

// UsageInfo holds token count info for metadata callbacks.
type UsageInfo struct {
	InputTokens        int
	InputTokenDetails  InputTokenDetails
	OutputTokens       int
	OutputTokenDetails OutputTokenDetails
	TotalTokens        int
}

type (
	InputTokenDetails  struct{ NoCacheTokens, CacheReadTokens, CacheWriteTokens int }
	OutputTokenDetails struct{ TextTokens, ReasoningTokens int }
)

// ToUIMessageStream converts an event iterator into a channel of typed Chunks.
// The returned channel is closed when the stream completes.
func ToUIMessageStream(events iter.Seq2[aikit.StepEvent, error], msgID string, opts ToUIStreamOptions) <-chan Chunk {
	eventCh := eventChannel(events)

	// Determine whether we need to intercept events.
	needIntercept := !opts.SendReasoning || !opts.SendSources || opts.MessageMetadata != nil

	filteredCh := eventCh
	// totalUsage is written by the interceptor goroutine and read when the finish
	// chunk is emitted by a different goroutine; a mutex guards it.
	var (
		totalUsage UsageInfo
		usage      usageAccumulator
		usageMu    sync.Mutex
	)

	if needIntercept {
		filteredCh = interceptEvents(eventCh, opts, &usage, &totalUsage, &usageMu)
	}

	producer := NewChunkProducer(msgID)
	cs := producer.Produce(filteredCh)

	sendStart := boolVal(opts.SendStart, true)
	sendFinish := boolVal(opts.SendFinish, true)
	needLifecycleFilter := !sendStart || !sendFinish

	// If no metadata callback and no lifecycle filtering, return chunks directly.
	if opts.MessageMetadata == nil && !needLifecycleFilter {
		return cs.Chunks
	}

	// Wrap to attach metadata and/or filter lifecycle chunks.
	return wrapChunksWithMetadata(cs.Chunks, opts, sendStart, sendFinish, &totalUsage, &usageMu)
}

// interceptEvents filters and tracks usage from the raw engine event stream.
// It writes accumulated usage into totalUsage (updated concurrently by the goroutine).
func interceptEvents(
	eventCh <-chan aikit.StepEvent,
	opts ToUIStreamOptions,
	usage *usageAccumulator,
	totalUsage *UsageInfo,
	usageMu *sync.Mutex,
) <-chan aikit.StepEvent {
	intercepted := make(chan aikit.StepEvent, 64)

	go func() {
		defer close(intercepted)
		defer recoverPanic(recoverToEvent(intercepted))
		for ev := range eventCh {
			if ev.Type == aikit.StepEventStepStart {
				usageMu.Lock()
				usage.startStep()
				usageMu.Unlock()
			}
			// Track usage for metadata.
			if ev.Type == aikit.StepEventUsage && ev.Usage != nil {
				usageMu.Lock()
				usage.apply(ev.Usage)
				*totalUsage = usage.snapshot()
				usageMu.Unlock()
			}
			// Filter reasoning events.
			if !opts.SendReasoning && ev.Type == aikit.StepEventReasoningDelta {
				continue
			}
			// Filter source events.
			if !opts.SendSources && ev.Type == aikit.StepEventSource {
				continue
			}
			intercepted <- ev
		}
	}()

	return intercepted
}

// wrapChunksWithMetadata wraps the chunk stream to attach metadata and/or filter lifecycle chunks.
func wrapChunksWithMetadata(
	chunks <-chan Chunk,
	opts ToUIStreamOptions,
	sendStart, sendFinish bool,
	totalUsage *UsageInfo,
	usageMu *sync.Mutex,
) <-chan Chunk {
	out := make(chan Chunk, 64)
	go func() {
		defer close(out)
		defer recoverPanic(recoverToTerminalChunks(out))
		var lastFinishReason string
		for c := range chunks {
			if !sendStart && c.Type == ChunkStart {
				continue
			}
			if !sendFinish && c.Type == ChunkFinish {
				continue
			}
			if c.Type == ChunkFinish && opts.MessageMetadata != nil {
				c = attachMessageMetadata(c, opts.MessageMetadata, lastFinishReason, totalUsage, usageMu)
			}
			if c.Type == ChunkFinish {
				if fr, ok := c.Fields["finishReason"].(string); ok {
					lastFinishReason = fr
				}
			}
			out <- c
		}
	}()
	return out
}

// attachMessageMetadata calls the metadata callback and injects the result into the finish chunk.
func attachMessageMetadata(
	c Chunk,
	metaFn func(MessageMetadataInfo) map[string]any,
	finishReason string,
	usage *UsageInfo,
	usageMu *sync.Mutex,
) Chunk {
	if fr, ok := c.Fields["finishReason"].(string); ok && fr != "" {
		finishReason = fr
	}
	// Snapshot usage under the lock so the callback never reads the shared value
	// while the interceptor goroutine is still accumulating.
	usageMu.Lock()
	usageSnapshot := *usage
	usageMu.Unlock()
	// Observer callback: a panic leaves metadata nil and the finish chunk is
	// still emitted.
	var metadata map[string]any
	safeObserver(func() {
		metadata = metaFn(MessageMetadataInfo{
			FinishReason: finishReason,
			Usage:        &usageSnapshot,
		})
	})
	if metadata != nil {
		if c.Fields == nil {
			c.Fields = make(map[string]any)
		}
		c.Fields["messageMetadata"] = metadata
	}
	return c
}
