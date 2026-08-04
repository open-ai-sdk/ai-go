package llm

import (
	"context"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// Model is the minimal contract implemented by language-model providers.
//
// Stream returns a channel, not an iter.Seq2, and that is deliberate. It is the
// provider-facing contract: every third-party provider implements it, so
// widening it would break all of them. The consumer-facing surface — StreamSend,
// StreamPrompt, StreamChat, and the agent's stream entrypoints — is uniformly
// iter.Seq2. The boundary is between who implements and who consumes, not an
// inconsistency.
//
// Cancelling the context is how a caller releases the stream. Implementations
// must close the channel when the context is done; consumers abandon whatever
// is left in it rather than draining, so a provider that ignores cancellation
// leaks its own goroutine.
type Model interface {
	ModelID() string
	Stream(context.Context, Request) (<-chan aikit.StreamEvent, error)
}

// CompletionModel is the optional native single-response capability. Models
// that only implement Model remain fully supported through stream aggregation.
// Implementations should retain their untranslated successful response in
// CompletionResponse.RawResponse.
type CompletionModel interface {
	Model
	Complete(context.Context, Request) (*CompletionResponse, error)
}
