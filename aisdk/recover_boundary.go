package aisdk

import (
	"fmt"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func recoverPanic(onPanic func(error)) {
	value := recover()
	if value == nil || onPanic == nil {
		return
	}
	var err error
	if recovered, ok := value.(error); ok {
		err = recovered
	} else {
		err = fmt.Errorf("panic: %v", value)
	}
	onPanic(err)
}

// recoverToChunk builds an onPanic that surfaces a recovered panic as a
// best-effort error chunk on out before the producer goroutine's deferred
// close runs. The send is non-blocking: a recovery path must never deadlock on
// a full or abandoned channel. aisdk has no logger injection yet, so the
// panic is surfaced to the consumer rather than logged.
func recoverToChunk(out chan<- Chunk) func(error) {
	return func(err error) {
		select {
		case out <- Chunk{Type: ChunkError, Fields: map[string]any{"errorText": "stream panic: " + err.Error()}}:
		default:
		}
	}
}

// recoverToEvent builds an onPanic that surfaces a recovered panic as a
// best-effort error event on out, for relay goroutines that carry engine
// events rather than chunks. Non-blocking for the same reason as recoverToChunk.
func recoverToEvent(out chan<- aikit.StepEvent) func(error) {
	return func(err error) {
		select {
		case out <- aikit.StepEvent{Type: aikit.StepEventError, Error: err}:
		default:
		}
	}
}

// safeObserver invokes an observer callback — one whose return value does not
// steer control flow — with a recovery boundary that swallows and continues.
// This mirrors node's mergeCallbacks, which runs callbacks under
// Promise.allSettled and swallows their errors: an observer panic must not tear
// down the UI stream.
func safeObserver(fn func()) {
	defer recoverPanic(nil)
	fn()
}
