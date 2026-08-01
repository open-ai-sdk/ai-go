package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
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
	ctx, cancel := context.WithCancel(context.Background())
	reader, writer := io.Pipe()
	defer writer.Close()
	body := &closeRecorder{ReadCloser: reader, closed: make(chan struct{})}
	resp := &http.Response{StatusCode: http.StatusOK, Body: body}

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
