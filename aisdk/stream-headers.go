package aisdk

import "net/http"

// UI message stream response headers, matching
// ai-v7-node/packages/ai/src/ui-message-stream/ui-message-stream-headers.ts exactly.
//
// All five matter:
//   - text/event-stream and no-cache are what make it a stream at all.
//   - x-vercel-ai-ui-message-stream: v1 is how the client identifies the protocol.
//   - x-accel-buffering: no stops nginx from buffering the whole response. It does
//     NOT stop Go's own bufio.Writer — that needs an explicit Flush per chunk, which
//     is SSEWriter's job.
const (
	HeaderContentType     = "content-type"
	HeaderCacheControl    = "cache-control"
	HeaderConnection      = "connection"
	HeaderUIMessageStream = "x-vercel-ai-ui-message-stream"
	HeaderAccelBuffering  = "x-accel-buffering"
)

// streamHeaders is the canonical name→value set.
var streamHeaders = [...]struct{ Name, Value string }{
	{HeaderContentType, "text/event-stream"},
	{HeaderCacheControl, "no-cache"},
	{HeaderConnection, "keep-alive"},
	{HeaderUIMessageStream, "v1"},
	{HeaderAccelBuffering, "no"},
}

// StreamHeaders returns the headers as a map, for callers that do not have an
// http.Header to write into.
func StreamHeaders() map[string]string {
	out := make(map[string]string, len(streamHeaders))
	for _, h := range streamHeaders {
		out[h.Name] = h.Value
	}
	return out
}

// SetStreamHeaders writes all five headers onto h.
//
// Uses Set rather than Add so a second call is idempotent: a handler wrapped by
// middleware that also sets content-type should end with one value, not two.
func SetStreamHeaders(h http.Header) {
	for _, hdr := range streamHeaders {
		h.Set(hdr.Name, hdr.Value)
	}
}
