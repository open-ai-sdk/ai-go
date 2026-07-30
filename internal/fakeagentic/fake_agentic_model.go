// Package fakeagentic provides a scripted model.BaseModel[*schema.AgenticMessage]
// for deterministic tests of the Eino→AI SDK v7 adapter.
//
// A real provider cannot be asserted against: block indices, emission order, and
// which sub-field of a delta is populated all vary per provider and per request.
// This fake makes those exact, so tests can pin the stream shape the producer must
// handle rather than the shape one provider happened to emit.
//
// It originated as the Phase 00 spike's fake and is the same code the spike
// measured Eino's behaviour with, so its emission shape is not invented — it
// mirrors agenticclaude's (event_convertor.go:254-326).
package fakeagentic

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// Turn is one scripted model response.
//
// Frames and Whole are authored independently on purpose: B8 asserts that
// schema.ConcatAgenticMessages over Frames reproduces Whole block-for-block, so
// deriving one from the other would make that test circular.
type Turn struct {
	// Frames are the streamed ContentBlock chunks in emission order. Each is sent
	// as a single-block AgenticMessage, matching the shape agenticclaude emits at
	// event_convertor.go:254-326.
	Frames []*schema.ContentBlock

	// Whole is the non-streaming response Generate returns for this turn.
	Whole *schema.AgenticMessage

	// StreamDelay is slept between frames, giving a mid-stream cancel (B4)
	// something to interrupt.
	StreamDelay time.Duration
}

// FakeAgenticModel implements model.BaseModel[*schema.AgenticMessage] with a
// scripted turn list. Turn N is served on the Nth call, so a ReAct loop that
// calls back after a tool result observes a different response and the loop's
// multi-turn behaviour becomes observable.
type FakeAgenticModel struct {
	Turns []Turn

	calls     atomic.Int64
	sent      atomic.Int64
	toolInfos atomic.Pointer[[]*schema.ToolInfo]
}

var _ model.BaseModel[*schema.AgenticMessage] = (*FakeAgenticModel)(nil)

// Calls reports how many times the agent invoked the model. A value above 1
// proves the agentic ReAct loop is multi-turn rather than single-shot.
func (m *FakeAgenticModel) Calls() int { return int(m.calls.Load()) }

// FramesSent reports how many stream frames were actually written. B4 compares
// this against the frame count the consumer saw to count post-cancel production.
func (m *FakeAgenticModel) FramesSent() int { return int(m.sent.Load()) }

// ToolInfos returns the tool definitions the agent passed via model.WithTools
// on the most recent call (adk/chatmodel.go:1482 is the wiring point).
func (m *FakeAgenticModel) ToolInfos() []*schema.ToolInfo {
	if p := m.toolInfos.Load(); p != nil {
		return *p
	}
	return nil
}

func (m *FakeAgenticModel) Generate(_ context.Context, _ []*schema.AgenticMessage,
	opts ...model.Option) (*schema.AgenticMessage, error) {
	turn, err := m.nextTurn(opts...)
	if err != nil {
		return nil, err
	}
	return turn.Whole, nil
}

func (m *FakeAgenticModel) Stream(ctx context.Context, _ []*schema.AgenticMessage,
	opts ...model.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	turn, err := m.nextTurn(opts...)
	if err != nil {
		return nil, err
	}

	sr, sw := schema.Pipe[*schema.AgenticMessage](1)
	go func() {
		defer sw.Close()
		for _, blk := range turn.Frames {
			// Checked even at zero delay so cancellation is observable on the
			// frame boundary rather than only at the end of the turn.
			if ctx.Err() != nil {
				return
			}
			if turn.StreamDelay > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(turn.StreamDelay):
				}
			}
			m.sent.Add(1)
			if sw.Send(&schema.AgenticMessage{
				Role:          schema.AgenticRoleTypeAssistant,
				ContentBlocks: []*schema.ContentBlock{blk},
			}, nil) {
				return // reader closed
			}
		}
	}()
	return sr, nil
}

// nextTurn advances the script and records the tools the agent supplied.
func (m *FakeAgenticModel) nextTurn(opts ...model.Option) (*Turn, error) {
	if infos := model.GetCommonOptions(&model.Options{}, opts...).Tools; len(infos) > 0 {
		m.toolInfos.Store(&infos)
	}
	n := int(m.calls.Add(1))
	if n > len(m.Turns) {
		return nil, fmt.Errorf("fake agentic model: turn %d requested, only %d scripted", n, len(m.Turns))
	}
	return &m.Turns[n-1], nil
}
