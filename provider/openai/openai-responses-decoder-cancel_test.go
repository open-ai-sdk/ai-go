package openai

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/ai"
)

// silentBody blocks Read until Close is called, modelling a Responses stream
// that opened but then went silent.
type silentBody struct {
	closed   chan struct{}
	once     sync.Once
	mu       sync.Mutex
	didClose bool
}

func newSilentBody() *silentBody { return &silentBody{closed: make(chan struct{})} }

func (b *silentBody) Read([]byte) (int, error) {
	<-b.closed
	return 0, io.EOF
}

func (b *silentBody) Close() error {
	b.once.Do(func() {
		b.mu.Lock()
		b.didClose = true
		b.mu.Unlock()
		close(b.closed)
	})
	return nil
}

func (b *silentBody) wasClosed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.didClose
}

// TestResponsesDecoder_CancelReturnsAndClosesBody drives the real Responses SSE
// decoder over a silent body and verifies that cancelling the context makes it
// return promptly and close the response body.
func TestResponsesDecoder_CancelReturnsAndClosesBody(t *testing.T) {
	body := newSilentBody()
	ch := make(chan ai.StreamEvent, 8)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		decodeResponsesSSEStream(ctx, body, ch)
	}()
	go func() {
		for range ch {
		}
	}()

	cancel()

	select {
	case <-done:
		if !body.wasClosed() {
			t.Error("expected the response body to be closed on cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Responses decoder did not return after context cancellation")
	}
}
