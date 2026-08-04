package aisdk

import (
	"io"
	"sync"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// ToolResult is a public-facing tool result notification emitted during streaming.
// Callers that need to react to tool results (e.g. emit document references) receive
// these via the ToolResultHook set on the Adapter.
type ToolResult struct {
	ToolCallID string
	ToolName   string
	ArgsJSON   string
	Output     string
}

// ToolResultHook is called after a tool result is emitted to the stream.
// wr is bound to the same io.Writer as the adapter so callers can emit additional chunks.
type ToolResultHook func(wr *Writer, result ToolResult)

// SourceHook is called when a source-url chunk is emitted during streaming.
// Callers can use this to collect grounding sources for persistence.
type SourceHook func(wr *Writer, sourceID, url, title string)

// Adapter translates an iterator of aikit.StepEvents into UI message stream chunks.
// It is transport-agnostic: callers can write to an http.ResponseWriter, a buffer, etc.
//
// For custom data-* chunks or source chunks between or after stream events,
// obtain the Writer via adapter.Writer(w) and call WriteData / WriteSourceURL directly.
type Adapter struct {
	msgID              string
	toolResultHook     ToolResultHook
	sourceHook         SourceHook
	onEnd              func(text, finishReason string)
	persistenceBuilder *PersistedMessageBuilder
}

// NewAdapter creates an Adapter with a fixed message ID.
func NewAdapter(msgID string) *Adapter {
	return &Adapter{msgID: msgID}
}

// WithToolResultHook sets a hook called after each tool result is emitted.
// Use this to emit custom side-channel data (e.g. document references) without
// wrapping the internal event channel.
func (a *Adapter) WithToolResultHook(hook ToolResultHook) *Adapter {
	a.toolResultHook = hook
	return a
}

// WithSourceHook sets a hook called when a source-url chunk is emitted.
// Use this to collect grounding sources for persistence or post-processing.
func (a *Adapter) WithSourceHook(hook SourceHook) *Adapter {
	a.sourceHook = hook
	return a
}

// WithOnEnd sets a callback invoked after the stream completes.
// text is the full accumulated assistant text; finishReason is "stop" or the
// finish reason captured from the last finish chunk.
func (a *Adapter) WithOnEnd(fn func(text, finishReason string)) *Adapter {
	a.onEnd = fn
	return a
}

// WithPersistenceBuilder sets a PersistedMessageBuilder that observes every chunk
// during Stream for typed-parts persistence.
func (a *Adapter) WithPersistenceBuilder(b *PersistedMessageBuilder) *Adapter {
	a.persistenceBuilder = b
	return a
}

// Writer returns a direct-write Writer bound to w for emitting custom chunks
// alongside a stream (e.g. WriteData, WriteSourceURL).
func (a *Adapter) Writer(w io.Writer) *Writer {
	return NewWriter(w)
}

// interceptState holds shared mutable state between the intercept goroutine and
// the main Stream loop, protected by a mutex.
type interceptState struct {
	mu        sync.Mutex
	usage     usageAccumulator
	toolCache map[string]toolData
}

// interceptEvents wraps an event channel to track usage and cache tool results.
// The returned channel forwards all events unchanged.
func (a *Adapter) interceptEvents(
	ch <-chan aikit.StepEvent,
	state *interceptState,
) <-chan aikit.StepEvent {
	intercepted := make(chan aikit.StepEvent, 64)
	go func() {
		defer close(intercepted)
		defer recoverPanic(recoverToEvent(intercepted))
		for ev := range ch {
			if ev.Type == aikit.StepEventStepStart {
				state.mu.Lock()
				state.usage.startStep()
				state.mu.Unlock()
			}
			if ev.Type == aikit.StepEventUsage && ev.Usage != nil {
				state.mu.Lock()
				state.usage.apply(ev.Usage)
				state.mu.Unlock()
			}
			if state.toolCache != nil && ev.Type == aikit.StepEventToolResult && ev.ToolResult != nil {
				tr := ev.ToolResult
				state.mu.Lock()
				state.toolCache[tr.ID] = toolData{
					toolName: tr.Name,
					argsJSON: tr.Args,
					output:   tr.Output,
				}
				state.mu.Unlock()
			}
			intercepted <- ev
		}
	}()
	return intercepted
}

// writeChunkWithHooks writes a non-finish, non-error chunk and fires registered
// hooks. It returns the chunk write error so the caller can stop writing once
// the client has disconnected; hook writes are the consumer's to handle.
func (a *Adapter) writeChunkWithHooks(wr *Writer, c Chunk, state *interceptState) error {
	if err := wr.WriteChunk(c.Type, c.Fields); err != nil {
		return err
	}

	if c.Type == ChunkSourceURL && a.sourceHook != nil {
		sid, ok1 := c.Fields["sourceId"].(string)
		surl, ok2 := c.Fields["url"].(string)
		stitle, ok3 := c.Fields["title"].(string)
		_, _, _ = ok1, ok2, ok3
		a.sourceHook(wr, sid, surl, stitle)
	}

	if c.Type == ChunkToolOutputAvailable && a.toolResultHook != nil {
		if tcID, ok := c.Fields["toolCallId"].(string); ok {
			state.mu.Lock()
			td, found := state.toolCache[tcID]
			state.mu.Unlock()
			if found {
				a.toolResultHook(wr, ToolResult{
					ToolCallID: tcID,
					ToolName:   td.toolName,
					ArgsJSON:   td.argsJSON,
					Output:     td.output,
				})
			}
		}
	}
	return nil
}

// toolData stores raw tool result strings keyed by toolCallID.
type toolData struct {
	toolName string
	argsJSON string
	output   string
}

// usageMetadata returns messageMetadata containing usage, or nil if no tokens were tracked.
func usageMetadata(u UsageInfo) map[string]any {
	if u.TotalTokens == 0 {
		return nil
	}
	return map[string]any{
		"usage": map[string]any{
			"inputTokens": u.InputTokens,
			"inputTokenDetails": map[string]any{
				"noCacheTokens":    u.InputTokenDetails.NoCacheTokens,
				"cacheReadTokens":  u.InputTokenDetails.CacheReadTokens,
				"cacheWriteTokens": u.InputTokenDetails.CacheWriteTokens,
			},
			"outputTokens": u.OutputTokens,
			"outputTokenDetails": map[string]any{
				"textTokens":      u.OutputTokenDetails.TextTokens,
				"reasoningTokens": u.OutputTokenDetails.ReasoningTokens,
			},
			"totalTokens": u.TotalTokens,
		},
	}
}

// Stream consumes events from ch and writes SSE lines to w.
// It returns the full concatenated assistant text for persistence.
