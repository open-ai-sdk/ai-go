package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
)

type closeRecorder struct {
	io.ReadCloser
	once   sync.Once
	closed chan struct{}
}

func (r *closeRecorder) Close() error {
	r.once.Do(func() {
		close(r.closed)
	})
	return r.ReadCloser.Close()
}

func TestStream_CancelClosesHungResponseAndChannel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.WriteHeader(http.StatusOK)
		writer.(http.Flusher).Flush()
		<-request.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req) //nolint:bodyclose // handed to Stream below.
	if err != nil {
		t.Fatal(err)
	}
	body := &closeRecorder{ReadCloser: resp.Body, closed: make(chan struct{})}
	resp.Body = body

	decode := func(
		_ context.Context,
		reader *SSEReader,
		events chan<- aikit.StreamEvent,
	) error {
		for {
			frame, readErr := reader.Next()
			if readErr != nil {
				return readErr
			}
			events <- aikit.StreamEvent{
				Type:      aikit.StreamEventTextDelta,
				TextDelta: frame.Data,
			}
		}
	}
	stream := Stream(ctx, resp, decode)
	cancel()

	select {
	case <-body.closed:
	case <-time.After(time.Second):
		t.Fatal("response body was not closed after cancellation")
	}

	var terminal error
	for event := range stream {
		if event.Type == aikit.StreamEventError {
			terminal = event.Error
		}
	}
	if terminal != nil && !errors.Is(terminal, context.Canceled) {
		t.Fatalf("terminal error = %v, want context cancellation", terminal)
	}
}
