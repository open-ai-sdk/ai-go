package agent

import (
	"errors"
	"iter"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

// StreamState reports how far a streamed run got. It is the model layer's enum
// rather than a second copy, so the two layers report terminal state in one
// vocabulary.
type StreamState = llm.StreamState

const (
	// StreamNotDrained means Events was never ranged, or is still ranging.
	StreamNotDrained = llm.StreamNotDrained
	// StreamCompleted means the run was consumed to its end, or the consumer
	// stopped on StepEventDone.
	StreamCompleted = llm.StreamCompleted
	// StreamAborted means the consumer stopped before StepEventDone, or the run
	// failed.
	StreamAborted = llm.StreamAborted
)

// ErrStreamNotDrained reports a read of the aggregate before the run's event
// sequence ended.
var ErrStreamNotDrained = errors.New("agent: run stream not drained")

// StepStream is one streamed agent run and the Result it aggregates while its
// events are consumed. Runner.Stream already drove a full reducer and discarded
// it; this surfaces what was already computed, so a caller needing both
// streaming output and a *Result no longer has to choose, and no extra model
// call is made.
//
// A StepStream is owned by a single goroutine, like llm.StreamingResponse:
// Events bodies run in the consumer's goroutine and that is where the aggregate
// is written. Result returns the internal aggregate, matching what Run returns,
// not a copy.
type StepStream struct {
	events  iter.Seq2[aikit.StepEvent, error]
	reducer *resultReducer
	state   StreamState
	err     error
}

// Events returns the single-use, single-owner event sequence. Breaking
// iteration cancels and drains the underlying runtime. A second range yields
// ErrStreamUsed and leaves the first range's aggregate intact.
func (s *StepStream) Events() iter.Seq2[aikit.StepEvent, error] { return s.events }

// Result returns the aggregate this run produced. It reports
// ErrStreamNotDrained before the event sequence has ended. When the run failed
// or the consumer stopped early, the partial Result is returned together with
// the terminal error.
func (s *StepStream) Result() (*Result, error) {
	if s.state == StreamNotDrained {
		return nil, ErrStreamNotDrained
	}
	return s.reducer.result, s.err
}

// State reports how far this run got. It is meaningful only to the goroutine
// that owns the stream.
func (s *StepStream) State() StreamState { return s.state }
