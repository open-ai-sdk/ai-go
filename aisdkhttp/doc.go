// Package aisdkhttp exposes the HTTP edge for AI SDK UI message streams.
//
// The package decodes v7 chat request envelopes, invokes a transport-agnostic
// event runner, and writes the resulting chunks as promptly flushed SSE. It
// intentionally has no dependency on providers or client-side transports.
//
// There are two entry points. Use Handler or HandlerFor, which hand the run
// just the decoded messages. Use HandlerForRequest when the run needs what else
// the decoder recovered — forwarded props, interrupt resume decisions, client
// tool declarations, run state — which the message-only form cannot reach.
// HandlerFor is an adapter over HandlerForRequest, so the two share one handler
// body and cannot drift.
package aisdkhttp
