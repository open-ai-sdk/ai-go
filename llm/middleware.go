package llm

import (
	"context"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// streamFunc is the Stream implementation a middleware substitutes.
type streamFunc func(context.Context, Request) (<-chan aikit.StreamEvent, error)

// wrappedModel is a Model whose Stream is replaced by middleware. It
// deliberately does not implement CompletionModel: Complete on the inner model
// would bypass the middleware, so callers fall back to stream aggregation,
// which goes through the wrapped Stream.
type wrappedModel struct {
	inner  Model
	stream streamFunc
}

func (m *wrappedModel) ModelID() string { return m.inner.ModelID() }

func (m *wrappedModel) Stream(ctx context.Context, req Request) (<-chan aikit.StreamEvent, error) {
	return m.stream(ctx, req)
}

// Unwrap returns the wrapped model.
func (m *wrappedModel) Unwrap() Model { return m.inner }

// wrappedCapableModel additionally forwards NativeSchemaSupport so structured
// output planning sees the real model's capability.
type wrappedCapableModel struct {
	wrappedModel
	capable NativeSchemaCapable
}

func (m *wrappedCapableModel) NativeSchemaSupport() NativeSchemaSupport {
	return m.capable.NativeSchemaSupport()
}

func wrapModel(inner Model, stream streamFunc) Model {
	base := wrappedModel{inner: inner, stream: stream}
	if capable, ok := inner.(NativeSchemaCapable); ok {
		return &wrappedCapableModel{wrappedModel: base, capable: capable}
	}
	return &base
}

// emitErr sends a terminal error event unless ctx is already done, then the
// caller closes the channel.
func emitErr(ctx context.Context, out chan<- aikit.StreamEvent, err error) {
	select {
	case out <- aikit.StreamEvent{Type: aikit.StreamEventError, Error: err}:
	case <-ctx.Done():
	}
}

// forward sends ev to out, giving up when ctx is done.
func forward(ctx context.Context, out chan<- aikit.StreamEvent, ev aikit.StreamEvent) bool {
	select {
	case out <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

// isContentEvent reports whether ev carries model output. Usage, finish, and
// error events are not content: a stream that has produced only those has not
// started answering.
func isContentEvent(ev aikit.StreamEvent) bool {
	switch ev.Type {
	case aikit.StreamEventTextDelta, aikit.StreamEventReasoningDelta,
		aikit.StreamEventToolCallDelta, aikit.StreamEventSource,
		aikit.StreamEventFileDelta:
		return true
	}
	return false
}
