package einoadapter

import (
	"errors"
	"io"
	"iter"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"github.com/open-ai-sdk/ai-go/aisdk"
)

// StepEventsFrom converts one Eino agent event into the normalized vocabulary the chunk
// producer consumes.
//
// The signature mirrors eino-ext/acp/conv.go's AgentEventToSessionUpdate: a pure function
// plus explicit stream state. State lives in ConvState rather than on the adapter so two
// concurrent runs cannot interleave their block bookkeeping.
//
// Streaming and non-streaming inputs must produce the SAME event sequence for equivalent
// content. The v7 client requires text-start before text-delta whether or not the server
// streamed, so a non-streaming message is expanded into the same start/delta/end triple
// rather than short-cutting to a single delta.
func StepEventsFrom(
	ev *adk.TypedAgentEvent[*schema.AgenticMessage],
	st *ConvState,
) iter.Seq2[aisdk.StepEvent, error] {
	return func(yield func(aisdk.StepEvent, error) bool) {
		if ev == nil {
			return
		}

		// A failed run is reported and abandoned. Continuing past an error would emit
		// content the model never produced.
		if ev.Err != nil {
			emitOpenBlockEnds(st, yield)
			yield(aisdk.StepEvent{Type: aisdk.StepEventError, Error: ev.Err}, nil)
			return
		}

		if ev.Action != nil && !emitAction(ev.Action, st, yield) {
			return
		}

		if ev.Output == nil || ev.Output.MessageOutput == nil {
			return
		}
		mo := ev.Output.MessageOutput

		// A user-role agentic message is a tool result being fed back to the model
		// (AgenticMessage has no `tool` role, so the tools node uses user-role messages).
		// Its blocks are still converted; only system-role input is skipped.
		if mo.AgenticRole == schema.AgenticRoleTypeSystem {
			return
		}

		if !mo.IsStreaming {
			convertWholeMessage(mo.Message, st, yield)
			return
		}
		convertStream(mo.MessageStream, st, yield)
	}
}

// emitAction maps an AgentAction. Returns false when conversion should stop.
func emitAction(a *adk.AgentAction, st *ConvState, yield func(aisdk.StepEvent, error) bool) bool {
	switch {
	case a.Interrupted != nil:
		// Surfaced opaquely for the approval gate. Any block still open would otherwise
		// never be closed, since the run stops here.
		if !emitOpenBlockEnds(st, yield) {
			return false
		}
		return yield(aisdk.StepEvent{
			Type: aisdk.StepEventInterrupted, Interrupt: a.Interrupted,
		}, nil)

	case a.Exit:
		if !emitOpenBlockEnds(st, yield) {
			return false
		}
		return yield(aisdk.StepEvent{
			Type: aisdk.StepEventRunFinish, FinishReason: aisdk.FinishReasonStop,
		}, nil)

	default:
		// TransferToAgent and BreakLoop are dropped deliberately: v7 has no multi-agent
		// transfer concept, and both are internal to the Eino run — the user-visible
		// stream is unaffected by which sub-agent produced a token.
		return true
	}
}

// convertWholeMessage expands a complete message into the same event shape a stream
// produces, so a non-streaming provider is indistinguishable on the wire.
func convertWholeMessage(
	msg *schema.AgenticMessage, st *ConvState, yield func(aisdk.StepEvent, error) bool,
) {
	if msg == nil {
		return
	}
	st.BeginTurn()

	for i, block := range msg.ContentBlocks {
		if block == nil {
			continue
		}
		// A complete message has no StreamingMeta, so position stands in for the index.
		// It only has to be unique within the turn, which it is.
		if !convertBlock(block, i, st, yield) {
			return
		}
	}
	if !emitOpenBlockEnds(st, yield) {
		return
	}
	emitUsage(msg, yield)
}

// convertStream drains a streamed message.
//
// The stream is closed unconditionally. The ADK contract says the MessageStream is
// exclusive and must be closed even when its events go unprocessed
// (adk/interface.go:458-462), and each one wraps a live provider response body.
func convertStream(
	sr *schema.StreamReader[*schema.AgenticMessage],
	st *ConvState, yield func(aisdk.StepEvent, error) bool,
) {
	if sr == nil {
		return
	}
	defer sr.Close()

	st.BeginTurn()
	var last *schema.AgenticMessage

	for {
		frame, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			emitOpenBlockEnds(st, yield)
			yield(aisdk.StepEvent{Type: aisdk.StepEventError, Error: err}, nil)
			return
		}
		if frame == nil {
			continue
		}
		last = frame

		for _, block := range frame.ContentBlocks {
			if block == nil {
				continue
			}
			if !convertBlock(block, blockIndexOf(block), st, yield) {
				return
			}
		}
	}

	if !emitOpenBlockEnds(st, yield) {
		return
	}
	emitUsage(last, yield)
}

// blockIndexOf reads the stream index a delta frame carries.
//
// StreamingMeta is the only identity a delta frame has. A tool-result block arrives with
// StreamingMeta nil (measured in the Phase 00 spike), which is fine: results are keyed by
// CallID, not by index, and index 0 for them is never used to open a block.
func blockIndexOf(b *schema.ContentBlock) int {
	if b.StreamingMeta != nil {
		return b.StreamingMeta.Index
	}
	return 0
}

// emitOpenBlockEnds closes every still-open block, in index order.
//
// This runs on every exit path — end of stream, error, interrupt — because the v7 client
// resets its active-part maps at finish-step. A block left open there can never be closed,
// and its part stays state:"streaming" forever.
func emitOpenBlockEnds(st *ConvState, yield func(aisdk.StepEvent, error) bool) bool {
	for _, idx := range st.OpenIndexes() {
		if !closeBlock(st, idx, yield) {
			return false
		}
	}
	return true
}

func closeBlock(st *ConvState, index int, yield func(aisdk.StepEvent, error) bool) bool {
	b := st.Block(index)
	if b == nil {
		return true
	}
	st.Close(index)

	switch b.kind {
	case blockText:
		return yield(aisdk.StepEvent{Type: aisdk.StepEventTextEnd, BlockID: b.id}, nil)

	case blockReasoning:
		ev := aisdk.StepEvent{Type: aisdk.StepEventReasoningEnd, BlockID: b.id}
		if b.signature != "" {
			// The signature belongs on reasoning-end's provider metadata. Some models
			// require it echoed back on the next turn, and it arrives as its own delta
			// with empty Text — so a "same kind means delta" rule would turn it into
			// reasoning-delta{delta:""} and lose it.
			ev.ThoughtSignature = b.signature
			ev.ProviderMetadata = map[string]any{"signature": b.signature}
		}
		return yield(ev, nil)

	case blockToolCall:
		return yield(aisdk.StepEvent{
			Type:             aisdk.StepEventToolCallReady,
			ToolCallID:       b.callID,
			ToolCallName:     b.toolName,
			ToolInput:        b.args.String(),
			ProviderExecuted: boolPtr(b.providerExecuted),
		}, nil)
	}
	return true
}

func emitUsage(msg *schema.AgenticMessage, yield func(aisdk.StepEvent, error) bool) {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.TokenUsage == nil {
		return
	}
	u := msg.ResponseMeta.TokenUsage
	yield(aisdk.StepEvent{
		Type: aisdk.StepEventUsage,
		Usage: &aisdk.Usage{
			InputTokens:  u.PromptTokens,
			OutputTokens: u.CompletionTokens,
			TotalTokens:  u.TotalTokens,
		},
	}, nil)
}

func boolPtr(b bool) *bool { return &b }
