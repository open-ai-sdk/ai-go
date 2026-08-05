package aisdkhttp

import (
	"context"
	"iter"
	"net/http"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/uistream"
	"github.com/open-ai-sdk/ai-go/uistream/ainode"
)

// RunFunc starts an agent run for the messages decoded by a UI protocol.
// Returning an error reports a pre-stream failure as HTTP 500. Errors emitted
// through the returned sequence are redacted and encoded as error chunks.
type RunFunc func(
	ctx context.Context,
	messages []aikit.Message,
) (iter.Seq2[aikit.StepEvent, error], error)

// RequestRunFunc starts an agent run with the whole decoded request, including
// the protocol extras a RunFunc cannot see: forwarded props, interrupt resume
// decisions, client tool declarations, and run state.
type RequestRunFunc func(
	ctx context.Context,
	request uistream.Request,
) (iter.Seq2[aikit.StepEvent, error], error)

// Handler returns an http.Handler for AI SDK v7 chat POSTs.
func Handler(run RunFunc) http.Handler {
	return HandlerFor(ainode.Protocol(), run)
}

// HandlerFor returns a handler driven by a UI stream protocol. Handler keeps
// the established AI Node v7 behavior by selecting ainode.Protocol.
//
// Use HandlerForRequest instead when the run needs anything the decoder
// recovered beyond the messages.
func HandlerFor(protocol uistream.Protocol, run RunFunc) http.Handler {
	if run == nil {
		panic("aisdkhttp: nil RunFunc")
	}
	// Adapting rather than duplicating the handler body keeps the two entry
	// points from drifting on status codes, headers, or cancellation.
	return HandlerForRequest(protocol, func(
		ctx context.Context,
		request uistream.Request,
	) (iter.Seq2[aikit.StepEvent, error], error) {
		return run(ctx, request.Messages)
	})
}

// HandlerForRequest is HandlerFor with the whole decoded request. HandlerFor
// remains the message-only form.
func HandlerForRequest(protocol uistream.Protocol, run RequestRunFunc) http.Handler {
	if run == nil {
		panic("aisdkhttp: nil RunFunc")
	}
	if protocol.Decoder == nil || protocol.Framer == nil || protocol.NewEncoder == nil {
		panic("aisdkhttp: incomplete protocol")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeHTTPError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		defer func() { _ = r.Body.Close() }()

		request, err := protocol.Decoder.Decode(r.Body)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, invalidRequestMessage)
			return
		}

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		events, err := run(ctx, request)
		if err != nil || events == nil {
			writeHTTPError(w, http.StatusInternalServerError, streamErrorMessage)
			return
		}

		protocol.Framer.ApplyHeaders(w.Header())
		writer := newFramingWriter(w, cancel)
		if err := uistream.Pipe(
			ctx,
			writer,
			events,
			protocol,
			uistream.Options{
				MessageID:    request.MessageID,
				Extra:        request.Extra,
				OnWriteError: func(error) { cancel() },
			},
		); err != nil {
			cancel()
			return
		}
	})
}
