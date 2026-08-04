package aisdk

import (
	"iter"
	"sync"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// mergeConfig holds options for Writer.Merge.
type mergeConfig struct {
	toolResultHook     ToolResultHook
	sourceHook         SourceHook
	onEnd              func(text string)
	persistenceBuilder *PersistedMessageBuilder
}

// MergeOption configures Writer.Merge behavior.
type MergeOption func(*mergeConfig)

// MergeWithToolResultHook sets a hook called after each tool result is emitted.
func MergeWithToolResultHook(hook ToolResultHook) MergeOption {
	return func(c *mergeConfig) {
		c.toolResultHook = hook
	}
}

// MergeWithSourceHook sets a callback invoked when a source-url chunk is emitted.
func MergeWithSourceHook(hook SourceHook) MergeOption {
	return func(c *mergeConfig) {
		c.sourceHook = hook
	}
}

// MergeWithOnEnd sets a callback invoked when the merged stream completes.
func MergeWithOnEnd(fn func(text string)) MergeOption {
	return func(c *mergeConfig) {
		c.onEnd = fn
	}
}

// Merge pipes events through this Writer using a temporary
// Adapter. Custom chunks can be written to wr before and after the call.
// Returns the full accumulated assistant text.
//
// Example:
//
//	wr := aisdk.NewWriter(sseWriter)
//	wr.WriteStart(msgID)
//	wr.WriteData("plan", planData)           // custom data before stream
//	text := wr.Merge(events)                 // model stream events
//	wr.WriteData("sources", sourcesData)     // custom data after stream
//	wr.WriteFinish()
//
// Note: Merge does NOT emit start or finish chunks; lifecycle
// management (WriteStart / WriteFinish) remains the caller's responsibility.
func (wr *Writer) Merge(events iter.Seq2[aikit.StepEvent, error], opts ...MergeOption) string {
	cfg := &mergeConfig{}
	for _, o := range opts {
		o(cfg)
	}

	// Use a bare ChunkProducer (no msgID) so it does not emit a duplicate start
	// chunk — the caller manages start/finish lifecycle.
	producer := newMergeProducer()

	ch := eventChannel(events)

	// If a tool result hook is set, intercept events before the producer.
	producerCh := ch
	type toolData struct {
		toolName string
		argsJSON string
		output   string
	}
	// toolCache is written by the interceptor goroutine below and read by the
	// main loop, so a mutex guards it — a concurrent map read/write is a fatal
	// runtime error that recover cannot catch.
	var (
		toolCache   map[string]toolData
		toolCacheMu sync.Mutex
	)
	if cfg.toolResultHook != nil {
		toolCache = make(map[string]toolData)
		intercepted := make(chan aikit.StepEvent, 64)
		go func() {
			defer close(intercepted)
			defer recoverPanic(recoverToEvent(intercepted))
			for ev := range ch {
				if ev.Type == aikit.StepEventToolResult && ev.ToolResult != nil {
					tr := ev.ToolResult
					toolCacheMu.Lock()
					toolCache[tr.ID] = toolData{
						toolName: tr.Name,
						argsJSON: tr.Args,
						output:   tr.Output,
					}
					toolCacheMu.Unlock()
				}
				intercepted <- ev
			}
		}()
		producerCh = intercepted
	}

	cs := producer.Produce(producerCh)

	// Once a write fails the client has disconnected; stop writing but keep
	// draining so the producer never blocks, and keep observing for persistence.
	var writeErr error
	for c := range cs.Chunks {
		if cfg.persistenceBuilder != nil {
			cfg.persistenceBuilder.ObserveChunk(c)
		}
		if writeErr != nil {
			continue
		}
		switch c.Type {
		case ChunkFinish:
			// Do NOT emit finish here; caller manages lifecycle.
		case ChunkStart:
			// Skip the start chunk emitted by the producer; caller owns lifecycle.
		case ChunkError:
			writeErr = wr.writeChunk(c)
		default:
			writeErr = wr.WriteChunk(c.Type, c.Fields)

			if c.Type == ChunkSourceURL && cfg.sourceHook != nil {
				sid, ok1 := c.Fields["sourceId"].(string)
				surl, ok2 := c.Fields["url"].(string)
				stitle, ok3 := c.Fields["title"].(string)
				_ = ok1
				_ = ok2
				_ = ok3
				cfg.sourceHook(wr, sid, surl, stitle)
			}

			if c.Type == ChunkToolOutputAvailable && cfg.toolResultHook != nil {
				tcID, ok := c.Fields["toolCallId"].(string)
				if ok {
					toolCacheMu.Lock()
					td, found := toolCache[tcID]
					toolCacheMu.Unlock()
					if found {
						cfg.toolResultHook(wr, ToolResult{
							ToolCallID: tcID,
							ToolName:   td.toolName,
							ArgsJSON:   td.argsJSON,
							Output:     td.output,
						})
					}
				}
			}
		}
	}

	text := cs.FullText()

	if cfg.onEnd != nil {
		cfg.onEnd(text)
	}

	return text
}

// newMergeProducer creates a ChunkProducer with an empty msgID since the
// merge flow does not emit start/finish lifecycle chunks.
func newMergeProducer() *ChunkProducer {
	return NewChunkProducer("")
}
