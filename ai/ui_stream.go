package ai

import (
	"context"
	"net/http"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/aisdk"
	"github.com/open-ai-sdk/ai-go/aisdkhttp"
)

// AgentStream runs a and returns a channel of UI-message-stream Chunks for
// msgID, closed when the run completes.
//
// AgentStream, AgentHandler, and PipeAgentStream are free functions over Agent.
// They live in the top-level façade so the lower-level aisdk wire package stays
// independent of models, agents, providers, and HTTP transports.
//
// These functions take already-converted ai.Message values via ai.WithMessages.
// AgentHandler also decodes the existing ChatRequestEnvelope wire format.
func AgentStream(ctx context.Context, a Agent, msgID string, opts ...Option) (<-chan aisdk.Chunk, error) {
	sr, err := a.Stream(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return aisdk.ToUIMessageStream(sr, msgID, aisdk.ToUIStreamOptions{}), nil
}

// PipeAgentStream runs a and writes its UI message stream as SSE onto w,
// returning the first write error encountered. w is flushed after every chunk
// (when it implements http.Flusher), so a disconnected client surfaces as a
// non-nil error instead of the run silently continuing against a writer
// nobody is reading from.
func PipeAgentStream(ctx context.Context, w http.ResponseWriter, a Agent, msgID string, opts ...Option) error {
	chunks, err := AgentStream(ctx, a, msgID, opts...)
	if err != nil {
		return err
	}
	return aisdkhttp.WriteStream(w, chunks)
}

// AgentHandler returns an http.Handler that decodes a ChatRequestEnvelope
// request body, runs a with the envelope's messages plus opts, and streams the
// result back as SSE. opts apply to every request; per-request data comes
// solely from the decoded envelope. Normal submissions receive a fresh
// assistant message ID; an explicit messageId is reused for regeneration.
func AgentHandler(a Agent, opts ...Option) http.Handler {
	return aisdkhttp.Handler(func(
		ctx context.Context,
		messages []aikit.Message,
	) (<-chan aikit.StepEvent, error) {
		callOpts := append(append([]Option{}, opts...), WithMessages(messages...))
		result, err := a.Stream(ctx, callOpts...)
		if err != nil {
			return nil, err
		}
		return result.Stream(), nil
	})
}
