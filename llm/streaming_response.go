package llm

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"sync/atomic"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// StreamState reports how far a StreamingResponse got.
type StreamState int

const (
	// StreamNotDrained means Events was never ranged, or is still ranging.
	StreamNotDrained StreamState = iota
	// StreamCompleted means the event sequence was consumed to its end, or the
	// consumer stopped on the provider's terminal event. It reports that
	// nothing failed, not that every event was seen — see Events.
	StreamCompleted
	// StreamAborted means the consumer stopped before the terminal event, the
	// context was cancelled, or the call failed.
	StreamAborted
)

var (
	// ErrStreamUsed reports a second attempt to range a single-use stream.
	ErrStreamUsed = errors.New("llm: completion stream already used")
	// ErrStreamNotDrained reports a read of the aggregate before the event
	// sequence ended.
	ErrStreamNotDrained = errors.New("llm: completion stream not drained")
)

// StreamingResponse is one streaming model call and the aggregate it builds
// while its events are consumed. It gives a streamed completion the same
// terminal state Send returns, without a second model call.
//
// The provider call starts on the first pull from Events, not at construction,
// so obtaining a response and never ranging it opens no connection. Close
// releases one that is never ranged.
//
// A StreamingResponse is owned by a single goroutine. Events bodies run in the
// consumer's goroutine and that is where the aggregate is written, so calling
// Response, State, or Close from another goroutine is a data race rather than a
// supported call — cancel the context passed to StreamSend instead. Response
// returns the internal aggregate, not a copy.
type StreamingResponse struct {
	model   Model
	request CompletionRequest
	parent  context.Context

	// ctx and cancel are derived in start, not at construction. Deriving them
	// eagerly would register a child on a cancellable parent that a response
	// which is never ranged and never closed keeps alive — a leak for exactly
	// the case lazy start exists to make free.
	ctx      context.Context
	cancel   context.CancelFunc
	events   <-chan aikit.StreamEvent
	started  bool
	startErr error

	used   atomic.Bool
	state  StreamState
	result *CompletionResponse
	err    error

	fold      aikit.ToolCallFold
	toolParts map[int]int
	sawFinish bool
}

func newStreamingResponse(ctx context.Context, model Model, request CompletionRequest) *StreamingResponse {
	return &StreamingResponse{
		model:     model,
		request:   request,
		parent:    ctx,
		result:    &CompletionResponse{Message: aikit.Message{Role: aikit.RoleAssistant}},
		toolParts: make(map[int]int),
	}
}

// Events returns the single-use event sequence and folds each event into the
// aggregate as it passes. A second range yields ErrStreamUsed.
//
// A provider error event is delivered through the error half of the sequence
// rather than as an event, matching Runner.Stream, and ends the sequence. The
// aggregate built so far stays readable through Response.
//
// Breaking out early folds only what was yielded. Stopping on
// StreamEventFinish is treated as a normal exit rather than a cancellation, but
// it does not guarantee a whole aggregate: several providers report usage after
// the finish event — OpenAI-compatible endpoints send it on a trailing chunk
// with no choices, and the Gemini native decoder does the same — so a consumer
// that breaks there loses those counts. Range to the end when the aggregate
// matters.
func (s *StreamingResponse) Events() iter.Seq2[aikit.StreamEvent, error] {
	return func(yield func(aikit.StreamEvent, error) bool) {
		if !s.used.CompareAndSwap(false, true) {
			yield(aikit.StreamEvent{}, ErrStreamUsed)
			return
		}
		if err := s.start(); err != nil {
			s.state = StreamAborted
			yield(aikit.StreamEvent{}, err)
			return
		}
		defer s.release()

		for {
			select {
			case <-s.ctx.Done():
				s.abort(s.ctx.Err())
				yield(aikit.StreamEvent{}, s.err)
				return
			case event, ok := <-s.events:
				if !ok {
					s.state = StreamCompleted
					return
				}
				if event.Type == aikit.StreamEventError {
					err := event.Error
					if err == nil {
						err = errors.New("llm: completion stream emitted a nil error")
					}
					s.abort(err)
					yield(aikit.StreamEvent{}, err)
					return
				}
				s.absorb(event)
				if !yield(event, nil) {
					// Stopping on the provider's terminal event is a normal
					// early exit, not a failure worth synthesizing an error
					// for. It does not promise a whole aggregate; trailing
					// usage events are lost either way.
					if s.sawFinish {
						s.state = StreamCompleted
						return
					}
					s.abort(context.Canceled)
					return
				}
			}
		}
	}
}

// Response returns the aggregate this call produced. It reports
// ErrStreamNotDrained before the event sequence has ended. When the call
// failed, the partial aggregate is returned together with the terminal error;
// a nil aggregate is paired only with a non-nil error.
func (s *StreamingResponse) Response() (*CompletionResponse, error) {
	if s.state == StreamNotDrained {
		return nil, ErrStreamNotDrained
	}
	if s.startErr != nil {
		return nil, s.startErr
	}
	return s.result, s.err
}

// State reports how far this response got. It is meaningful only to the
// goroutine that owns the response.
func (s *StreamingResponse) State() StreamState { return s.state }

// Close makes a later Events yield ErrStreamUsed and cancels the provider call
// if one is in flight. Ranging to the end or breaking out already releases the
// stream, so Close is then a no-op, and a response that was never ranged has
// nothing to release — the point of Close there is to retire it so nothing
// starts a call later.
//
// Close belongs to the owning goroutine, like Response and State. It is not an
// out-of-band cancel handle for a range running elsewhere; cancel the context
// passed to StreamSend for that.
func (s *StreamingResponse) Close() error {
	s.used.Store(true)
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

// start opens the provider stream on the first pull. Request validation stays
// synchronous in StreamSend; only the provider call is deferred to here.
func (s *StreamingResponse) start() error {
	if s.started {
		return s.startErr
	}
	s.started = true
	s.ctx, s.cancel = context.WithCancel(s.parent)

	stream, err := s.model.Stream(s.ctx, s.request)
	if err != nil {
		s.cancel()
		s.startErr = wrapCompletionError(err, CompletionErrorKindProvider, "stream")
		return s.startErr
	}
	if stream == nil {
		s.cancel()
		s.startErr = invalidCompletionResponse("stream", "model returned a nil stream")
		return s.startErr
	}
	s.events = stream
	return nil
}

// release writes the folded tool calls into the aggregate and cancels the
// provider call. Cancellation is the whole release mechanism: Model.Stream owns
// its channel and closes it on cancel, so the events left in it are abandoned
// rather than drained. Draining would let a provider that ignores cancellation
// block the caller forever, which is why neither collectCompletion nor the
// agent's stream consumer ever did it.
func (s *StreamingResponse) release() {
	s.flushToolCalls()
	s.cancel()
}

func (s *StreamingResponse) abort(err error) {
	s.state = StreamAborted
	s.err = err
}

func (s *StreamingResponse) absorb(event aikit.StreamEvent) {
	response := s.result
	switch event.Type {
	case aikit.StreamEventTextDelta:
		response.Text += event.TextDelta
		response.Message.Content = aikit.AppendText(
			response.Message.Content, event.TextDelta, event.ThoughtSignature,
		)
	case aikit.StreamEventReasoningDelta:
		response.Reasoning += event.TextDelta
		response.Message.Content = aikit.AppendReasoning(
			response.Message.Content, event.TextDelta, event.ThoughtSignature,
		)
	case aikit.StreamEventToolCallDelta:
		// The part is reserved where the first delta arrived so tool calls keep
		// their position among interleaved text; the fold fills it on release.
		if isNew := s.fold.Add(event); isNew {
			s.toolParts[event.ToolCallIndex] = len(response.Message.Content)
			response.Message.Content = append(
				response.Message.Content, aikit.ContentPart{Type: aikit.ContentPartTypeToolCall},
			)
		}
	case aikit.StreamEventUsage:
		if event.Usage != nil {
			response.Usage = response.Usage.Merge(*event.Usage)
		}
	case aikit.StreamEventSource:
		if event.Source != nil {
			response.Sources = append(response.Sources, *event.Source)
		}
	case aikit.StreamEventFileDelta:
		if len(event.FileData) != 0 {
			data := append([]byte(nil), event.FileData...)
			response.Files = append(response.Files, GeneratedFile{Data: data, MediaType: event.FileMediaType})
			response.Message.Content = append(response.Message.Content, aikit.ContentPart{
				Type:      aikit.ContentPartTypeFile,
				Data:      append([]byte(nil), data...),
				MediaType: event.FileMediaType,
			})
		}
	case aikit.StreamEventFinish:
		s.sawFinish = true
		response.MessageID = event.MessageID
		response.Message.ID = event.MessageID
		response.FinishReason = event.FinishReason
		response.RawFinishReason = event.RawFinishReason
		response.ProviderMetadata = cloneMap(event.ProviderMetadata)
		response.Warnings = append(response.Warnings, event.Warnings...)
	}
}

func (s *StreamingResponse) flushToolCalls() {
	for _, draft := range s.fold.Completed() {
		index, reserved := s.toolParts[draft.Index]
		if !reserved {
			continue
		}
		part := &s.result.Message.Content[index]
		part.ToolCallID = draft.ID
		part.ToolCallName = draft.Name
		part.ThoughtSignature = draft.ThoughtSignature
		if draft.Args != "" {
			part.ToolCallArgs = json.RawMessage(draft.Args)
		}
	}
}
