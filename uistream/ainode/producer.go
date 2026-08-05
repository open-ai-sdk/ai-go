package ainode

import (
	"log/slog"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// Chunk is a typed UI stream chunk that can be serialized to different transports.
type Chunk struct {
	Type   string         // chunk type constant (ChunkTextDelta, etc.)
	Fields map[string]any // chunk-specific payload fields
}

// ChunkStream is a stream of typed chunks produced from engine events.
// Drain Chunks before calling FullText.
type ChunkStream struct {
	// Chunks is the output channel; it is closed when the producer is done.
	Chunks <-chan Chunk

	done chan struct{}
	text string
}

// FullText blocks until the Chunks channel is fully drained and returns the
// accumulated assistant text.
func (cs *ChunkStream) FullText() string {
	<-cs.done
	return cs.text
}

// ChunkProducer translates aikit.StepEvents into a channel of typed Chunks.
// It holds the same per-stream state as the former Adapter internals.
type ChunkProducer struct {
	msgID string

	// per-step state — reset on each StepStart
	textBlockID      string
	textBlockCount   int
	textStarted      bool
	reasoningStarted bool
	toolInputStarted map[string]bool
	toolInputReady   map[string]bool
	toolArgsAccum    map[string]string
	checker          *InvariantChecker
	reporter         func(InvariantViolation)
	logger           *slog.Logger
	// usage folds per-step snapshots into a run total. v7 has no usage chunk
	// field, so the total is published through messageMetadata on finish.
	usage usageAccumulator

	// lastFinishReason stores the finish reason from the most recent StepEventStepEnd.
	lastFinishReason string
	// lastThoughtSignature stores the most recent thought signature from a reasoning delta.
	lastThoughtSignature string
}

// NewChunkProducer creates a ChunkProducer with the given message ID.
type ChunkProducerOption func(*ChunkProducer)

func WithInvariantReporter(reporter func(InvariantViolation)) ChunkProducerOption {
	return func(producer *ChunkProducer) { producer.reporter = reporter }
}

func WithInvariantLogger(logger *slog.Logger) ChunkProducerOption {
	return func(producer *ChunkProducer) { producer.logger = logger }
}

func NewChunkProducer(msgID string, options ...ChunkProducerOption) *ChunkProducer {
	producer := &ChunkProducer{
		msgID:            msgID,
		toolInputStarted: make(map[string]bool),
		toolInputReady:   make(map[string]bool),
		toolArgsAccum:    make(map[string]string),
		checker:          NewInvariantChecker(),
	}
	for _, option := range options {
		option(producer)
	}
	return producer
}

// Produce starts consuming events from ch, emitting Chunks to the returned
// ChunkStream. The returned ChunkStream.Chunks channel is closed when ch is
// exhausted or an error event is received.
//
// Produce is designed for single use per ChunkProducer instance.
func (cp *ChunkProducer) Produce(ch <-chan aikit.StepEvent) *ChunkStream {
	out := make(chan Chunk, 64)
	cs := &ChunkStream{
		Chunks: out,
		done:   make(chan struct{}),
	}

	go func() {
		defer close(out)
		defer close(cs.done)
		defer recoverPanic(recoverToTerminalChunks(out))
		defer func() {
			for _, violation := range cp.checker.Finalize() {
				reportInvariant(cp.logger, cp.reporter, violation)
			}
		}()

		// Emit the stream-start chunk first.
		cp.emit(out, Chunk{Type: ChunkStart, Fields: map[string]any{"messageId": cp.msgID}})

		for ev := range ch {
			if ev.Type == aikit.StepEventError {
				for _, c := range cp.chunksError(ev.Error) {
					cp.emit(out, c)
				}
				return
			}
			chunks, delta := cp.translateEvent(ev)
			for _, c := range chunks {
				cp.emit(out, c)
			}
			cs.text += delta
		}
	}()

	return cs
}

func (cp *ChunkProducer) emit(out chan<- Chunk, chunk Chunk) {
	for _, violation := range cp.checker.Observe(chunk) {
		reportInvariant(cp.logger, cp.reporter, violation)
	}
	out <- chunk
}

// translateEvent converts a single StepEvent into zero or more Chunks plus any
// text delta accumulated.
func (cp *ChunkProducer) translateEvent(ev aikit.StepEvent) ([]Chunk, string) {
	switch ev.Type {
	case aikit.StepEventStepStart:
		cp.usage.startStep()
		return cp.chunksStepStart(), ""
	case aikit.StepEventUsage:
		cp.usage.apply(ev.Usage)
		return nil, ""
	case aikit.StepEventTextDelta:
		return cp.chunksTextDelta(ev)
	case aikit.StepEventReasoningDelta:
		return cp.chunksReasoningDelta(ev), ""
	case aikit.StepEventToolCallStart:
		if ev.ToolCallID == "" {
			reportInvariant(cp.logger, cp.reporter, InvariantViolation{
				Code: InvariantEmptyToolCallID, ChunkType: ChunkToolInputStart, Field: "toolCallId",
			})
		}
		return cp.chunksToolCallStart(ev), ""
	case aikit.StepEventToolCallDelta:
		return cp.chunksToolCallDelta(ev), ""
	case aikit.StepEventToolCallReady:
		return cp.chunksToolCallReady(ev), ""
	case aikit.StepEventToolResult:
		return cp.chunksToolResult(ev), ""
	default:
		return cp.translateTerminalEvent(ev)
	}
}

func (cp *ChunkProducer) translateTerminalEvent(ev aikit.StepEvent) ([]Chunk, string) {
	switch ev.Type {
	case aikit.StepEventToolApprovalRequest:
		approvalID := ev.ApprovalID
		if approvalID == "" {
			approvalID = ev.ToolCallID
		}
		fields := map[string]any{
			"approvalId": approvalID,
			"toolCallId": ev.ToolCallID,
			"toolName":   ev.ToolCallName,
			"args":       ev.ToolCallArgsDelta,
		}
		// isAutomatic and signature are optional in the protocol; include them
		// only when set.
		if ev.ApprovalIsAutomatic {
			fields["isAutomatic"] = true
		}
		if ev.ApprovalSignature != "" {
			fields["signature"] = ev.ApprovalSignature
		}
		return []Chunk{{Type: ChunkToolApprovalRequest, Fields: fields}}, ""
	case aikit.StepEventToolOutputDenied:
		return []Chunk{{Type: ChunkToolOutputDenied, Fields: map[string]any{"toolCallId": ev.ToolCallID}}}, ""
	case aikit.StepEventToolCallInvalid:
		return cp.chunksToolCallInvalid(ev), ""
	case aikit.StepEventSource:
		return cp.chunksSource(ev), ""
	case aikit.StepEventFileDelta:
		return cp.chunksFile(ev), ""
	case aikit.StepEventStructuredOutput:
		return cp.chunksStructuredOutput(ev), ""
	case aikit.StepEventStepEnd:
		if ev.FinishReason == "" {
			cp.lastFinishReason = ""
			return cp.chunksStepEnd(), ""
		}
		finishReason, ok := wireFinishReason(ev.FinishReason)
		if !ok {
			finishReason = "other"
		}
		cp.lastFinishReason = finishReason
		return cp.chunksStepEnd(), ""
	case aikit.StepEventClientToolRequest:
		// No chunk: in AI SDK v7 a tool call with no result followed by a clean
		// finish already *is* the client-tool contract, and the surrounding
		// TOOL_CALL_* events produced it. AG-UI needs an explicit interrupt
		// because its client only executes frontend tools from one.
		return nil, ""
	case aikit.StepEventStateSnapshot, aikit.StepEventStateDelta:
		// AI SDK v7 has no run-state channel. Dropping these is the honest
		// mapping; a server that needs shared state uses the AG-UI adapter.
		return nil, ""
	case aikit.StepEventDone:
		fields := map[string]any{}
		if cp.lastFinishReason != "" {
			fields["finishReason"] = cp.lastFinishReason
		}
		if metadata := cp.usageMetadata(); metadata != nil {
			fields["messageMetadata"] = metadata
		}
		return []Chunk{
			{Type: ChunkFinish, Fields: fields},
		}, ""
	}
	return nil, ""
}
