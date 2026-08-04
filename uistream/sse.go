package uistream

import (
	"fmt"
	"io"
	"net/http"
)

// SSEFramer writes conventional server-sent events.
type SSEFramer struct{}

func (SSEFramer) ApplyHeaders(h http.Header) {
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
}
func (SSEFramer) WriteFrame(w io.Writer, f Frame) error {
	if f.Name != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", f.Name); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "data: %s\n\n", f.Data)
	return err
}
