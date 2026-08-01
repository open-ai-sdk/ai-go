package ai

import (
	"context"
	"net/http"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/aisdk"
	"github.com/open-ai-sdk/ai-go/aisdkhttp"
)

// AgentStream runs a and returns a channel of UI-message-stream Chunks for
// msgID, closed when the run completes. It is the free-function equivalent of
// ai-sdk-node's createAgentUIStream.
//
// AgentStream, AgentHandler, and PipeAgentStream are free functions over Agent.
// They live in the top-level façade so the lower-level aisdk wire package stays
// independent of models, agents, providers, and HTTP transports.
//
// Node-parity gap, deliberately not built here (its own plan; see package
// ai's Agent doc comment for the related tool-typing gap): the
// validateUIMessages → convertToModelMessages → originalMessages chain
// (ai-sdk-node's createAgentUIStream resolves an agent's ModelMessage input
// from arbitrary UI messages via that chain). Neither function exists in
// ai-go yet, so these three functions take already-converted ai.Message
// values via ai.WithMessages — AgentHandler's one concession is decoding the
// existing ChatRequestEnvelope wire format, which predates this phase and is
// already used by non-agent handlers.
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
// nobody is reading from — phase 7 made every UI-stream writer report
// failures this way, and an agent run is no exception.
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
