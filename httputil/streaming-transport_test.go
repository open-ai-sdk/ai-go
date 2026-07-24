package httputil

import (
	"context"
	"sync"
	"testing"
	"time"
)

// recordingCloser records whether Close was called.
type recordingCloser struct {
	mu     sync.Mutex
	closed bool
}

func (c *recordingCloser) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *recordingCloser) wasClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// TestDefaultStreamingTransport_NoClientTimeout verifies the shared client sets
// transport-level deadlines but no overall client timeout.
func TestDefaultStreamingTransport_NoClientTimeout(t *testing.T) {
	client := NewStreamingClient(5 * time.Second)
	if client.Timeout != 0 {
		t.Errorf("streaming client must have no overall Timeout, got %v", client.Timeout)
	}
	transport := DefaultStreamingTransport(5 * time.Second)
	if transport.ResponseHeaderTimeout != 5*time.Second {
		t.Errorf("ResponseHeaderTimeout = %v, want 5s", transport.ResponseHeaderTimeout)
	}
}

// TestCloseOnCancel_ClosesOnCancel verifies the watcher closes the target when
// the context is cancelled.
func TestCloseOnCancel_ClosesOnCancel(t *testing.T) {
	c := &recordingCloser{}
	ctx, cancel := context.WithCancel(context.Background())
	stop := CloseOnCancel(ctx, c)
	defer stop()

	cancel()

	deadline := time.Now().Add(time.Second)
	for !c.wasClosed() {
		if time.Now().After(deadline) {
			t.Fatal("CloseOnCancel did not close the target after cancellation")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestCloseOnCancel_StopPreventsClose verifies that stopping the watcher before
// cancellation means the target is never closed by the watcher.
func TestCloseOnCancel_StopPreventsClose(t *testing.T) {
	c := &recordingCloser{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := CloseOnCancel(ctx, c)
	stop() // normal completion

	// Give the watcher a moment to observe the stop signal, then cancel.
	time.Sleep(10 * time.Millisecond)
	cancel()
	time.Sleep(10 * time.Millisecond)

	if c.wasClosed() {
		t.Error("watcher closed the target after being stopped")
	}
}
