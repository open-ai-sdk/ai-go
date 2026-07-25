package uistream

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"

	"github.com/open-ai-sdk/ai-go/ai"
)

// AgentStream runs a and returns a channel of UI-message-stream Chunks for
// msgID, closed when the run completes. It is the free-function equivalent of
// ai-sdk-node's createAgentUIStream.
//
// AgentStream, AgentHandler, and PipeAgentStream are free functions over
// ai.Agent — never methods on the agent, and never a type named
// uistream.Stream (no such type exists). That placement is deliberate: today
// uistream depends on ai (ten files import it), and ai depends on nothing
// under uistream. A method on *ai.ToolLoopAgent returning a uistream type
// would force the reverse dependency and cycle with httputil, which already
// imports both.
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
func AgentStream(ctx context.Context, a ai.Agent, msgID string, opts ...ai.Option) (<-chan Chunk, error) {
	sr, err := a.Stream(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return ToUIMessageStream(sr, msgID, ToUIStreamOptions{}), nil
}

// PipeAgentStream runs a and writes its UI message stream as SSE onto w,
// returning the first write error encountered. w is flushed after every chunk
// (when it implements http.Flusher), so a disconnected client surfaces as a
// non-nil error instead of the run silently continuing against a writer
// nobody is reading from — phase 7 made every UI-stream writer report
// failures this way, and an agent run is no exception.
func PipeAgentStream(ctx context.Context, w http.ResponseWriter, a ai.Agent, msgID string, opts ...ai.Option) error {
	chunks, err := AgentStream(ctx, a, msgID, opts...)
	if err != nil {
		return err
	}
	return WriteSSEStream(newFlushingSSEWriter(w), chunks)
}

// AgentHandler returns an http.Handler that decodes a ChatRequestEnvelope
// request body, runs a with the envelope's messages plus opts, and streams the
// result back as SSE. opts apply to every request; per-request data comes
// solely from the decoded envelope (message-ID resolution via
// ResolveMessageIDFromEnvelope, falling back to a freshly generated ID when
// the envelope carries none to resume) — this covers the common "new turn"
// case, not message continuation (see the package comment).
func AgentHandler(a ai.Agent, opts ...ai.Option) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env ChatRequestEnvelope
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		msgID := ResolveMessageIDFromEnvelope(env, newMessageID())
		callOpts := append(append([]ai.Option{}, opts...), ai.WithMessages(ToAIMessages(env.Messages)...))
		// A custom Agent whose Stream fails before any bytes are written gets a
		// proper 500; ToolLoopAgent never errors here (failures ride the stream).
		chunks, err := AgentStream(r.Context(), a, msgID, callOpts...)
		if err != nil {
			http.Error(w, "stream error", http.StatusInternalServerError)
			return
		}
		_ = WriteSSEStream(newFlushingSSEWriter(w), chunks)
	})
}

// newMessageID generates a random assistant message identifier for a request
// that carries no continuation ID to resume. Every other uistream entry point
// takes msgID from the caller (there is no ID-generator utility in ai-go);
// this is the one case — a fresh HTTP request with nothing to resume — where
// there is genuinely nothing for the caller to supply instead.
func newMessageID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "msg"
	}
	return "msg_" + hex.EncodeToString(b)
}

// flushingSSEWriter wraps an http.ResponseWriter so every Write flushes
// immediately, mirroring httputil.NewSSEWriter. It is duplicated here rather
// than imported: httputil already imports uistream, and importing httputil
// back would cycle.
type flushingSSEWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func newFlushingSSEWriter(w http.ResponseWriter) io.Writer {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("x-vercel-ai-ui-message-stream", "v1")
	f, _ := w.(http.Flusher)
	return &flushingSSEWriter{w: w, f: f}
}

func (s *flushingSSEWriter) Write(p []byte) (int, error) {
	n, err := s.w.Write(p)
	if err == nil && s.f != nil {
		s.f.Flush()
	}
	return n, err
}
