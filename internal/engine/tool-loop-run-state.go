package engine

import (
	"context"
	"log/slog"

	"github.com/open-ai-sdk/ai-go/internal/safego"
)

// run carries the per-run context and output channel so that every event send
// in the tool loop goes through a single ctx-guarded path. This is what bounds
// cancellation latency to one in-flight event: a cancelled context unblocks any
// pending send instead of leaving a goroutine parked on a full channel.
type run struct {
	ctx    context.Context
	out    chan<- StepEvent
	logger *slog.Logger
}

// safeObserver invokes an observer callback (one whose return value does not
// steer the loop) with a recovery boundary that logs and continues. This mirrors
// node's mergeCallbacks, which runs callbacks under Promise.allSettled and
// swallows their errors — an observer panic must not fail the run.
func (r *run) safeObserver(fn func()) {
	if fn == nil {
		return
	}
	defer safego.Recover(r.logger, nil, "callback", "observer")
	fn()
}

// emit delivers ev to the consumer unless the context is cancelled first. A
// false return means the consumer is gone (ctx cancelled); the caller must
// unwind without attempting further sends. Because cancellation means "stop",
// dropping the remaining events on a false return is correct, not a truncation
// bug — the consumer asked to stop receiving.
func (r *run) emit(ev StepEvent) bool {
	select {
	case r.out <- ev:
		return true
	case <-r.ctx.Done():
		return false
	}
}
