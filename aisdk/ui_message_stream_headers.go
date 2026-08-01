package uistream

import "net/http"

// SetUIMessageStreamHeaders applies the headers required by the AI SDK UI
// message stream protocol.
func SetUIMessageStreamHeaders(h http.Header) {
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("x-vercel-ai-ui-message-stream", "v1")
	h.Set("x-accel-buffering", "no")
}
