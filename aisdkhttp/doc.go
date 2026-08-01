// Package aisdkhttp exposes the HTTP edge for AI SDK UI message streams.
//
// The package decodes v7 chat request envelopes, invokes a transport-agnostic
// event runner, and writes the resulting chunks as promptly flushed SSE. It
// intentionally has no dependency on providers or client-side transports.
package aisdkhttp
