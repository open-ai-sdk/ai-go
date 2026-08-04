package aikit

import (
	"context"
	"iter"
)

// Stream is a single-use event sequence. E is the layer's event vocabulary:
// StreamEvent for one model call, StepEvent for a multi-turn agent run.
type Stream[E any] interface {
	Events() iter.Seq2[E, error]
}

// StreamingPrompt streams a single prompt with no history.
//
// S is the concrete stream type, not the Stream interface. Go has no covariant
// return types: an interface method declared to return Stream[E] can only be
// satisfied by a method whose return type is literally Stream[E], which erases
// the concrete type and forces every caller to type-assert before it can reach
// the aggregate the stream carries. Carrying S as a second parameter keeps the
// concrete return while E stays bound for generic consumers.
//
// The cost is partial inference. A generic helper over these interfaces infers
// the implementer but not E or S, so its callers must name them:
//
//	drain[aikit.StreamEvent, *llm.StreamingResponse](ctx, handle, "hello")
//
// Direct method calls are unaffected.
type StreamingPrompt[E any, S Stream[E]] interface {
	StreamPrompt(ctx context.Context, prompt string) (S, error)
}

// StreamingChat streams a single prompt after history. History is copied into
// the request; it is never mutated.
type StreamingChat[E any, S Stream[E]] interface {
	StreamChat(ctx context.Context, prompt string, history ...Message) (S, error)
}

// StreamingCompletion returns a request builder rather than a stream, so the
// caller can shape the request before sending it. B is the layer's builder
// type.
type StreamingCompletion[B any] interface {
	StreamCompletion(ctx context.Context, prompt string, history ...Message) (B, error)
}
