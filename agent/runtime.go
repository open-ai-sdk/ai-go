package agent

import (
	"context"
	"log/slog"
	"sync"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/internal/safego"
	"github.com/open-ai-sdk/ai-go/internal/tracing"
)

// run carries the per-run context and output channel so that every event send
// in the tool loop goes through a single ctx-guarded path. This is what bounds
// cancellation latency to one in-flight event: a cancelled context unblocks any
// pending send instead of leaving a goroutine parked on a full channel.
type run struct {
	ctx                 context.Context
	out                 chan<- StepEvent
	logger              *slog.Logger
	callbacks           *lifecycleCallbacks
	approvalKey         []byte
	approvalReplayGuard ApprovalReplayGuard
	// tracer is never nil: runLoop substitutes tracing.NoopTracer{} when the
	// caller configured none, so every call site can use it unconditionally.
	tracer tracing.Tracer
	// tracingEnabled is false exactly when the caller configured no Tracer at
	// all (tracer is then the NoopTracer fallback). Call sites use it to skip
	// building a span's attribute slice rather than build one and hand it to
	// a tracer that discards it: constructing a []tracing.Attr and passing it
	// through the Tracer interface allocates unconditionally — the compiler
	// cannot see through the dynamic dispatch to prove NoopTracer never
	// retains it — so this flag is what keeps the fully-disabled path
	// allocation-free instead of merely no-op.
	tracingEnabled bool
	// traceContent mirrors runConfig.TraceContent — whether spans may carry
	// prompt/completion/tool-argument content.
	traceContent bool
	maxTurns     int
	modelCalls   int
	enforceTurns bool
	hooks        []Hook
	hookContext  HookContext
	hookMu       sync.Mutex
	hookErr      error
	// These are the prepared request values for the current model turn. They
	// are snapshotted into every invocation context before a tool starts.
	toolsContext   aikit.ToolsContext
	runtimeContext aikit.RuntimeContext
}

func (r *run) reserveModelCall() error {
	if r.enforceTurns && r.modelCalls >= r.maxTurns {
		return &MaxTurnsError{MaxTurns: r.maxTurns}
	}
	r.modelCalls++
	return nil
}

// safeObserver invokes an observer callback (one whose return value does not
// steer the loop) with a recovery boundary that logs and continues. An observer
// panic must not fail the run.
func (r *run) safeObserver(fn func()) {
	safeObserver(r.logger, fn)
}

func safeObserver(logger *slog.Logger, fn func()) {
	if fn == nil {
		return
	}
	defer safego.Recover(logger, nil, "callback", "observer")
	fn()
}

func notifyError(logger *slog.Logger, callbacks *lifecycleCallbacks, err error) {
	if callbacks == nil || callbacks.OnError == nil {
		return
	}
	safeObserver(logger, func() { callbacks.OnError(err) })
}

func (r *run) emitError(err error) bool {
	if !r.emitObserved(StepEvent{Type: StepEventError, Error: err}) {
		return false
	}
	notifyError(r.logger, r.callbacks, err)
	return true
}

func (r *run) stopError() error {
	r.hookMu.Lock()
	defer r.hookMu.Unlock()
	if r.hookErr != nil {
		return r.hookErr
	}
	return r.ctx.Err()
}

func (r *run) setHookError(err error) {
	if err != nil && r.hookErr == nil {
		r.hookErr = err
	}
}

// observeEvent invokes event hooks in registration order. The stack is
// serialized so parallel tool paths cannot interleave two event callbacks.
func (r *run) observeEvent(ev StepEvent) bool {
	r.hookMu.Lock()
	defer r.hookMu.Unlock()
	if r.hookErr != nil {
		return false
	}
	for _, hook := range r.hooks {
		capability, ok := hook.(StreamEventHook)
		if !ok {
			continue
		}
		var err error
		r.safeObserver(func() {
			err = capability.OnStreamEvent(
				r.ctx,
				r.hookContext,
				snapshotStepEvent(ev),
			)
		})
		if err != nil {
			r.setHookError(hookFailure(hook, "stream_event", err))
			return false
		}
	}
	return true
}

// emit delivers ev to the consumer unless the context is cancelled first. A
// false return means the consumer is gone (ctx cancelled); the caller must
// unwind without attempting further sends. Because cancellation means "stop",
// dropping the remaining events on a false return is correct, not a truncation
// bug — the consumer asked to stop receiving.
func (r *run) emit(ev StepEvent) bool {
	if r.ctx.Err() != nil {
		return false
	}
	if !r.observeEvent(ev) {
		return false
	}
	select {
	case r.out <- snapshotStepEvent(ev):
		return true
	case <-r.ctx.Done():
		return false
	}
}

// emitObserved delivers a runtime-generated event and mirrors it to OnChunk.
// Provider stream events use applyStreamEvent, which already performs the same
// callback dispatch and therefore must continue to call emit directly.
func (r *run) emitObserved(ev StepEvent) bool {
	if !r.emit(ev) {
		return false
	}
	if r.callbacks != nil && r.callbacks.OnChunk != nil {
		callbackEvent := snapshotStepEvent(ev)
		r.safeObserver(func() { r.callbacks.OnChunk(callbackEvent) })
	}
	return true
}
