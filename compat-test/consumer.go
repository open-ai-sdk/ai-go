// Package compattest is an external consumer of github.com/open-ai-sdk/ai-go.
// Every type it names comes from the public ai and aitypes packages; it imports
// nothing under internal/. Before the public aitypes package existed, none of
// this compiled: LanguageModel.Stream and NewStreamResult referenced
// internal/engine types that an outside caller could not name.
package compattest

import (
	"context"

	"github.com/open-ai-sdk/ai-go/ai"
	"github.com/open-ai-sdk/ai-go/aitypes"
	"github.com/open-ai-sdk/ai-go/uistream"
)

// fakeModel is a hand-written ai.LanguageModel built entirely from the public
// surface. The compile-time interface assertion below is the proof.
type fakeModel struct{}

func (fakeModel) ModelID() string { return "fake" }

func (fakeModel) Stream(_ context.Context, _ ai.LanguageModelRequest) (<-chan ai.StreamEvent, error) {
	ch := make(chan ai.StreamEvent, 2)
	ch <- ai.StreamEvent{Type: ai.StreamEventTextDelta, TextDelta: "hello"}
	ch <- aitypes.StreamEvent{Type: aitypes.StreamEventFinish, FinishReason: aitypes.FinishReasonStop}
	close(ch)
	return ch, nil
}

// Compile-time proof an external consumer can implement the model interface.
var _ ai.LanguageModel = fakeModel{}

// fakeStreamEventer implements uistream.StreamEventer from outside the module,
// proving the UI-stream surface is mockable — its Stream() returns the public
// aitypes.StepEvent, not an unnameable internal type.
type fakeStreamEventer struct{ ch <-chan aitypes.StepEvent }

func (f fakeStreamEventer) Stream() <-chan aitypes.StepEvent { return f.ch }
func (fakeStreamEventer) DrainUnused()                       {}

// Compile-time proof an external consumer can implement the uistream surface.
var _ uistream.StreamEventer = fakeStreamEventer{}

// ConsumeFakeStream builds a StepEvent channel by hand and drives it through
// ai.NewStreamResult — both the channel element type and NewStreamResult's
// parameter were unnameable outside the module before aitypes was made public.
func ConsumeFakeStream() (string, error) {
	ch := make(chan aitypes.StepEvent, 3)
	ch <- aitypes.StepEvent{Type: aitypes.StepEventStepStart}
	ch <- aitypes.StepEvent{Type: aitypes.StepEventTextDelta, TextDelta: "hi"}
	ch <- aitypes.StepEvent{Type: aitypes.StepEventStepEnd, FinishReason: aitypes.FinishReasonStop}
	close(ch)

	sr := ai.NewStreamResult(ch) // parameter type is aitypes.StepEvent via the ai.StepEvent alias
	res, err := sr.Consume()
	if err != nil {
		return "", err
	}
	return res.Text, nil
}
