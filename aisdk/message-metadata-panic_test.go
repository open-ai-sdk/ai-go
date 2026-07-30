package aisdk

import (
	"sync"
	"testing"
	"time"
)

// TestWrapChunksWithMetadata_CallbackPanic_StreamContinues verifies that a panic
// in the MessageMetadata observer callback is swallowed (node mergeCallbacks
// parity): the finish chunk is still emitted, without metadata attached, and the
// output channel closes normally instead of the producer goroutine crashing.
func TestWrapChunksWithMetadata_CallbackPanic_StreamContinues(t *testing.T) {
	in := make(chan Chunk, 1)
	in <- Chunk{Type: ChunkFinish, Fields: map[string]any{"finishReason": "stop"}}
	close(in)

	var usage UsageInfo
	var mu sync.Mutex
	opts := ToUIStreamOptions{
		MessageMetadata: func(MessageMetadataInfo) map[string]any { panic("metadata boom") },
	}

	out := wrapChunksWithMetadata(in, opts, true, true, &usage, &mu)

	var got []Chunk
	timeout := time.After(5 * time.Second)
	for {
		select {
		case c, ok := <-out:
			if !ok {
				goto done
			}
			got = append(got, c)
		case <-timeout:
			t.Fatal("stream did not complete after a metadata-callback panic")
		}
	}
done:
	if len(got) != 1 {
		t.Fatalf("expected exactly one chunk, got %d", len(got))
	}
	if got[0].Type != ChunkFinish {
		t.Fatalf("expected a finish chunk, got %q", got[0].Type)
	}
	if _, ok := got[0].Fields["messageMetadata"]; ok {
		t.Error("metadata must be absent when the callback panics")
	}
}
