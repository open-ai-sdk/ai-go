package gemini

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/ai"
)

// blockingBody blocks Read until Close is called, modelling a provider that has
// sent headers but then goes silent mid-stream.
type blockingBody struct {
	closed   chan struct{}
	once     sync.Once
	mu       sync.Mutex
	didClose bool
}

func newBlockingBody() *blockingBody { return &blockingBody{closed: make(chan struct{})} }

func (b *blockingBody) Read([]byte) (int, error) {
	<-b.closed
	return 0, io.EOF
}

func (b *blockingBody) Close() error {
	b.once.Do(func() {
		b.mu.Lock()
		b.didClose = true
		b.mu.Unlock()
		close(b.closed)
	})
	return nil
}

func (b *blockingBody) wasClosed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.didClose
}

// TestNativeSSEDecoder_CancelUnblocksAndClosesBody verifies that cancelling the
// context unblocks a decoder stuck on a silent body and closes that body.
func TestNativeSSEDecoder_CancelUnblocksAndClosesBody(t *testing.T) {
	body := newBlockingBody()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range nativeTestStream(ctx, body) {
		}
	}()

	cancel()

	select {
	case <-done:
		if !body.wasClosed() {
			t.Error("expected the body to be closed on cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("decoder did not return after context cancellation (blocked read not interrupted)")
	}
}

// TestNativeSSEDecoder_LargeDataLineParses verifies a single data: line larger
// than the old 1 MB Scanner cap parses instead of failing with ErrTooLong.
func TestNativeSSEDecoder_LargeDataLineParses(t *testing.T) {
	big := strings.Repeat("a", 2*1024*1024) // 2 MB, well over the old 1 MB cap
	payload := `data: {"candidates":[{"content":{"parts":[{"text":"` + big + `"}]}}]}` + "\n"
	body := io.NopCloser(strings.NewReader(payload))

	var text string
	for ev := range nativeTestStream(context.Background(), body) {
		if ev.Type == ai.StreamEventError {
			t.Fatalf("unexpected error decoding a large line: %v", ev.Error)
		}
		if ev.Type == ai.StreamEventTextDelta {
			text += ev.TextDelta
		}
	}
	if len(text) != len(big) {
		t.Errorf("text length = %d, want %d (large line was truncated or dropped)", len(text), len(big))
	}
}
