package ai

import (
	"errors"
	"sync"

	"github.com/open-ai-sdk/ai-go/internal/safego"
)

// ErrStreamConsumed is delivered to a view registered after the source stream
// has already been fully consumed. Channel views surface it as a trailing
// StepEventError; Events() yields it. A late view is never a silently-closed
// empty channel.
var ErrStreamConsumed = errors.New("ai: stream already fully consumed")

// StreamResult wraps a streaming response with convenient accessors. It fans the
// single source channel out to any number of views — Stream, TextStream, Events,
// and Consume — created on demand.
//
// Views may be created in any order, before or after consumption starts. Each
// registers its own branch of the fan-out; a branch created after the source has
// advanced receives events from that point on, and one created after the source
// is exhausted receives ErrStreamConsumed rather than an empty channel.
//
// Backpressure: the source advances at the speed of the slowest *live* branch
// (each branch has a bounded buffer). A branch whose consumer has explicitly
// stopped — an Events() range that breaks — is unregistered and dropped, so it
// cannot wedge the source or the other views. Channel views (Stream/TextStream)
// have no break signal; their consumers are expected to drain to completion.
type StreamResult struct {
	src   <-chan StepEvent
	tools *ToolSet

	mu       sync.Mutex
	branches []*streamBranch
	started  bool
	srcDone  bool

	// done is closed when the fan-out goroutine has finished.
	done chan struct{}
}

// streamBranch is one registered view of the fan-out. done is closed when the
// branch unregisters so the producer's select drops it instead of blocking.
type streamBranch struct {
	ch   chan StepEvent
	done chan struct{}
}

// NewStreamResult wraps an engine step-event channel in a StreamResult.
func NewStreamResult(ch <-chan StepEvent) *StreamResult {
	return NewStreamResultWithTools(ch, nil)
}

// NewStreamResultWithTools wraps an engine step-event channel and preserves the
// tool definitions required to build response.messages in Consume().
func NewStreamResultWithTools(ch <-chan StepEvent, tools *ToolSet) *StreamResult {
	return &StreamResult{src: ch, tools: tools, done: make(chan struct{})}
}

// register creates a new branch of the fan-out. When the source is already
// exhausted it returns a branch pre-loaded with an ErrStreamConsumed event and a
// non-nil error, so every view surfaces the condition rather than a silent close.
func (sr *StreamResult) register() (*streamBranch, error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if sr.srcDone {
		b := &streamBranch{ch: make(chan StepEvent, 1), done: make(chan struct{})}
		b.ch <- StepEvent{Type: StepEventError, Error: ErrStreamConsumed}
		close(b.ch)
		return b, ErrStreamConsumed
	}
	b := &streamBranch{ch: make(chan StepEvent, 64), done: make(chan struct{})}
	sr.branches = append(sr.branches, b)
	return b, nil
}

// unregister removes b from sr's fan-out and signals the producer to drop it. It
// is safe to call more than once. The branch channel is intentionally not closed
// here: its consumer has already stopped reading, and finish() only closes
// branches still registered.
func (b *streamBranch) unregister(sr *StreamResult) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	for i, cur := range sr.branches {
		if cur == b {
			sr.branches = append(sr.branches[:i], sr.branches[i+1:]...)
			close(b.done)
			return
		}
	}
}

// ensureStarted launches the fan-out goroutine exactly once.
func (sr *StreamResult) ensureStarted() {
	sr.mu.Lock()
	if sr.started {
		sr.mu.Unlock()
		return
	}
	sr.started = true
	sr.mu.Unlock()

	go func() {
		defer sr.finish()
		defer safego.Recover(nil, func(err error) { sr.broadcast(StepEvent{Type: StepEventError, Error: err}) })
		for ev := range sr.src {
			sr.broadcast(ev)
		}
	}()
}

// broadcast delivers ev to every live branch. A slow but live branch applies
// backpressure (blocking send on its bounded buffer); an unregistered branch is
// dropped via its done channel.
func (sr *StreamResult) broadcast(ev StepEvent) {
	sr.mu.Lock()
	branches := append([]*streamBranch(nil), sr.branches...)
	sr.mu.Unlock()
	for _, b := range branches {
		select {
		case b.ch <- ev:
		case <-b.done:
		}
	}
}

// finish marks the source exhausted and closes every still-registered branch.
func (sr *StreamResult) finish() {
	sr.mu.Lock()
	sr.srcDone = true
	branches := sr.branches
	sr.branches = nil
	sr.mu.Unlock()
	for _, b := range branches {
		close(b.ch)
	}
	close(sr.done)
}

// TextStream returns a channel yielding text deltas only. Closed when the stream
// completes. A stream already consumed yields a closed channel (a text view has
// no error channel; use Stream or Events to observe ErrStreamConsumed).
func (sr *StreamResult) TextStream() <-chan string {
	b, err := sr.register()
	out := make(chan string, 64)
	if err != nil {
		close(out)
		return out
	}
	sr.ensureStarted()
	go func() {
		defer close(out)
		defer safego.Recover(nil, nil)
		defer b.unregister(sr)
		for ev := range b.ch {
			if ev.Type == StepEventTextDelta {
				out <- ev.TextDelta
			}
		}
	}()
	return out
}

// Stream returns the raw engine StepEvent channel, closed when the stream
// completes. This is an escape hatch for callers such as uistream.Adapter that
// need full event visibility.
//
// Unlike TextStream/Events, this channel has no break signal: abandoning it
// without draining leaves the branch registered, so once its buffer fills the
// producer blocks — stalling every other live view of the same result. Drain it
// to completion, or cancel the StreamText context to tear the whole run down.
func (sr *StreamResult) Stream() <-chan StepEvent {
	b, err := sr.register()
	if err == nil {
		sr.ensureStarted()
	}
	return b.ch
}

// DrainUnused exists for backward compatibility and to satisfy the
// uistream.StreamEventer interface. With the on-demand fan-out it is a no-op:
// unrequested views are never created, so there is nothing to drain. Safe to
// call any number of times, in any order relative to the view methods.
func (sr *StreamResult) DrainUnused() {}

// ConsumeStream drives the stream to completion without exposing any output,
// for fire-and-forget usage where side effects happen via callbacks. It
// registers one discard branch and drains it in the background.
func (sr *StreamResult) ConsumeStream() {
	b, err := sr.register()
	if err != nil {
		return
	}
	sr.ensureStarted()
	go func() {
		defer safego.Recover(nil, nil)
		defer b.unregister(sr)
		for range b.ch {
		}
	}()
}

// Consume blocks until the stream completes and returns the aggregated result.
// It reads its own branch so it does not interfere with other views.
func (sr *StreamResult) Consume() (*GenerateTextResult, error) {
	b, err := sr.register()
	if err == nil {
		sr.ensureStarted()
	}
	defer b.unregister(sr)

	result := &GenerateTextResult{}
	var currentStep *StepOutput

	for ev := range b.ch {
		switch ev.Type {
		case StepEventStepStart:
			currentStep = &StepOutput{}

		case StepEventTextDelta:
			result.Text += ev.TextDelta
			if currentStep != nil {
				currentStep.Text += ev.TextDelta
			}

		case StepEventReasoningDelta:
			result.Reasoning += ev.ReasoningDelta
			if currentStep != nil {
				currentStep.Reasoning += ev.ReasoningDelta
			}

		case StepEventToolCallStart:
			handleToolCallStart(ev, currentStep)

		case StepEventToolCallDelta:
			handleToolCallDelta(ev, currentStep)

		case StepEventToolCallReady:
			handleToolCallReady(ev, currentStep)

		case StepEventToolResult:
			currentStep = handleToolResult(ev, result, currentStep)

		case StepEventUsage:
			handleUsage(ev, result, currentStep)

		case StepEventSource:
			handleSource(ev, result, currentStep)

		case StepEventFileDelta:
			handleFileDelta(ev, result, currentStep)

		case StepEventStepEnd:
			currentStep = handleStepEnd(ev, result, currentStep, sr.tools)

		case StepEventStructuredOutput:
			result.StructuredOutput = ev.StructuredOutput

		case StepEventToolApprovalRequest, StepEventToolOutputDenied, StepEventToolCallInvalid:
			// Observed for streaming/UI consumers; they carry no aggregate state
			// into the non-streaming result, so Consume records nothing here.

		case StepEventError:
			return result, ev.Error
		}
	}

	result.Response = Response{Messages: ResponseMessagesForSteps(result.Steps, sr.tools)}
	return result, nil
}
