package einoadapter

import (
	"fmt"
	"sort"
	"strings"
)

// This file must not import Eino. It holds the streaming bookkeeping the conversion
// needs, and keeping it framework-free is what makes the flat *schema.Message path an
// addition rather than a second implementation of the same state machine. CI asserts the
// import boundary.

// blockKind is what a stream index is currently carrying.
type blockKind int

const (
	blockNone blockKind = iota
	blockText
	blockReasoning
	blockToolCall
)

func (k blockKind) String() string {
	switch k {
	case blockText:
		return "text"
	case blockReasoning:
		return "reasoning"
	case blockToolCall:
		return "tool-call"
	default:
		return "none"
	}
}

// blockState tracks one open block, keyed by its stream index.
//
// Keyed by index and never by call id, which is not a style preference. Only the FIRST
// frame of a tool-call block carries identity: the start frame has
// FunctionToolCall{CallID, Name}, and every delta frame is
// NewContentBlockChunk(&FunctionToolCall{Arguments: partialJSON}, meta) with CallID and
// Name empty. StreamingMeta has exactly one field, Index. A call-id-keyed accumulator
// therefore misses on every delta and emits tool-input-delta{toolCallId: ""}, which the
// v7 client throws on.
type blockState struct {
	kind blockKind
	// id is the composed, message-unique block id for text and reasoning blocks.
	id string
	// callID and toolName are captured from a tool-call block's start frame.
	callID   string
	toolName string
	// args accumulates partial-JSON fragments so ToolCallReady can carry the whole input.
	args strings.Builder
	// signature accumulates a reasoning signature, which arrives as its own delta with
	// empty Text and must end up on reasoning-end's provider metadata rather than being
	// flattened into an empty reasoning-delta.
	signature string
	// providerExecuted marks a provider-run tool, whose arguments never stream.
	providerExecuted bool
}

// ConvState is the per-run conversion state. One per agent run, never shared.
type ConvState struct {
	// turnOrdinal disambiguates block ids across model turns.
	//
	// Eino's StreamingMeta.Index is scoped to a single model response, and the Phase 00
	// spike measured that it restarts at 0 on the next turn: turn 1's tool call and turn
	// 2's text both arrived at index 0. The v7 wire needs ids unique across the whole
	// message, so index alone would make the second turn's text overwrite the first
	// turn's part — and the client does not throw for that, it just renders wrong.
	turnOrdinal int

	blocks map[int]*blockState
}

func NewConvState() *ConvState {
	return &ConvState{blocks: map[int]*blockState{}}
}

// BeginTurn starts a new model turn. Any block still open from the previous turn is the
// caller's problem to flush first — CloseAll exists for that — because emitting the *End
// events is the converter's job, not the state's.
func (s *ConvState) BeginTurn() {
	s.turnOrdinal++
	s.blocks = map[int]*blockState{}
}

// TurnOrdinal reports the current turn, 1-based after the first BeginTurn.
func (s *ConvState) TurnOrdinal() int { return s.turnOrdinal }

// BlockID composes a message-unique id from the turn and the stream index.
func (s *ConvState) BlockID(index int) string {
	return fmt.Sprintf("%d-%d", s.turnOrdinal, index)
}

// SynthesizeCallID invents a tool call id for a provider-run tool that did not supply one.
//
// ServerToolCall.CallID is documented as "Empty if not provided by the model server".
// Two web_search calls in one turn would then share the empty id, and the v7 client would
// collapse them into one rendered part without complaining — getToolInvocation matches ""
// against whichever part it finds. Synthesizing from turn and index keeps them distinct.
func (s *ConvState) SynthesizeCallID(index int) string {
	return fmt.Sprintf("srv-%d-%d", s.turnOrdinal, index)
}

// Block returns the state at index, or nil.
func (s *ConvState) Block(index int) *blockState { return s.blocks[index] }

// Open records a new block at index.
func (s *ConvState) Open(index int, kind blockKind) *blockState {
	b := &blockState{kind: kind, id: s.BlockID(index)}
	s.blocks[index] = b
	return b
}

// Close forgets the block at index. The caller emits the matching *End event.
func (s *ConvState) Close(index int) { delete(s.blocks, index) }

// OpenIndexes lists open blocks in ascending index order.
//
// Ordered because the flush at end-of-stream emits one *End per open block, and a
// map-iteration order would make the event sequence — and therefore the wire bytes —
// differ between runs of the same input.
func (s *ConvState) OpenIndexes() []int {
	out := make([]int, 0, len(s.blocks))
	for i := range s.blocks {
		out = append(out, i)
	}
	sort.Ints(out)
	return out
}

// HasOpen reports whether any block is still open.
func (s *ConvState) HasOpen() bool { return len(s.blocks) > 0 }
