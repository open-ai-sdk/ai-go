package ainode

import (
	"io"
	"iter"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// UIStreamOption configures StreamToWriter behavior.
type UIStreamOption func(*uiStreamBridgeConfig)

// uiStreamBridgeConfig holds options for StreamToWriter.
type uiStreamBridgeConfig struct {
	toolResultHook     ToolResultHook
	sourceHook         SourceHook
	onEnd              func(text string)
	persistenceBuilder *PersistedMessageBuilder
}

// WithUIToolResultHook sets a callback invoked after each tool result is emitted.
func WithUIToolResultHook(hook ToolResultHook) UIStreamOption {
	return func(c *uiStreamBridgeConfig) {
		c.toolResultHook = hook
	}
}

// WithUISourceHook sets a callback invoked when a source-url chunk is emitted.
// Use this to collect grounding sources for persistence.
func WithUISourceHook(hook SourceHook) UIStreamOption {
	return func(c *uiStreamBridgeConfig) {
		c.sourceHook = hook
	}
}

// WithUIOnEnd sets a callback invoked when the stream completes.
// text is the full accumulated assistant response.
func WithUIOnEnd(fn func(text string)) UIStreamOption {
	return func(c *uiStreamBridgeConfig) {
		c.onEnd = fn
	}
}

// WithUIPersistence sets a PersistedMessageBuilder that observes every chunk
// during streaming for typed-parts persistence.
func WithUIPersistence(b *PersistedMessageBuilder) UIStreamOption {
	return func(c *uiStreamBridgeConfig) {
		c.persistenceBuilder = b
	}
}

// StreamToWriter writes SSE UI message stream chunks to w, consuming all events
// from events. It returns the full assistant text for persistence.
//
// msgID is the assistant message identifier emitted in the start chunk.
// Callers may pass UIStreamOption values to attach hooks.
func StreamToWriter(
	events iter.Seq2[aikit.StepEvent, error],
	w io.Writer,
	msgID string,
	opts ...UIStreamOption,
) string {
	cfg := &uiStreamBridgeConfig{}
	for _, o := range opts {
		o(cfg)
	}

	adapter := NewAdapter(msgID)

	if cfg.toolResultHook != nil {
		adapter.WithToolResultHook(cfg.toolResultHook)
	}

	if cfg.sourceHook != nil {
		adapter.WithSourceHook(cfg.sourceHook)
	}

	if cfg.onEnd != nil {
		fn := cfg.onEnd
		adapter.WithOnEnd(func(text, _ string) {
			fn(text)
		})
	}

	if cfg.persistenceBuilder != nil {
		adapter.WithPersistenceBuilder(cfg.persistenceBuilder)
	}

	return adapter.Stream(events, w)
}
